package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"

	"filippo.io/age"
	"github.com/gin-gonic/gin"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
)

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode
var SKeys SecretKeys

const indexFile = "post.json"
const metaFile = "meta.json"

func StartWeb(ipfsAPI icore.CoreAPI, ipfsNode *core.IpfsNode, skeys SecretKeys) {
	IpfsAPI = ipfsAPI
	IpfsNode = ipfsNode
	SKeys = skeys

	router := gin.Default()

	router.GET("/ipns/:name/*path", ipnsHandler)
	router.GET("/ipfs/:cid/*path", ipfsHandler)
	router.POST("/publish", publishHandler)
	router.POST("/addipfskey", addIpfsKeyHandler)
	router.GET("/listipfskey", listIpfsKeyHandler)
	router.POST("/newsecretkey", newSecretKeyHandler)
	router.GET("/getsecretkey", getSecretKeyHandler)
	router.GET("/index", indexHandler)

	router.Run(":8088")
}

func ipnsHandler(c *gin.Context) {
	name := c.Param("name")
	fpath := c.Param("path")

	log.Println("fullpath ", name+fpath)

	fullPath, err := IpfsAPI.Name().Resolve(context.Background(), name+fpath)

	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}

	//pin
	pin := "success"
	err = IpfsAPI.Pin().Add(context.Background(), fullPath)
	if err != nil {
		pin = err.Error()
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, pin))
}

func ipfsHandler(c *gin.Context) {
	cid := c.Param("cid")
	fpath := c.Param("path")

	nd, err := IpfsAPI.Unixfs().Get(context.Background(), path.New(cid+fpath))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("err %s", err.Error())))
		return
	}
	defer nd.Close()

	fs := []string{}
	files.Walk(nd, func(fpath string, nd files.Node) error {
		fs = append(fs, fpath)
		return nil
	})

	//如果是单文件，就以文件处理，如果多文件，就认为是目录，列出所有文件
	if len(fs) != 1 {
		c.JSON(http.StatusOK, fs)
		return
	}

	f := files.ToFile(nd)
	if f == nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, "not a file"))
		return
	}

	//在这里解密
	//如果不是密闻，Decrypt会自动判断，返回原始数据
	o := bytes.NewBuffer([]byte{})
	err = Decrypt(SKeys.Identities, f, o)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("decrypt err %s", err.Error())))
		return
	}
	data := o.Bytes()

	c.Data(http.StatusOK, http.DetectContentType(data), data)

}

func publishHandler(c *gin.Context) {
	log.Println("publish start")
	/// --- 1. 定义一个files.Nodes map,用于上传到IPFS网络
	postMap := map[string]files.Node{}

	//从请求中提取出请求内容
	var postform postForm
	err := c.ShouldBind(&postform)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("bind err: %s", err.Error())))
		return
	}

	/// --- 2. 获得组织加密公钥

	//如果postform.To 有值，那么加上自己的 secretkey ,否则自己是看不到自己发的消息的
	tos := []age.Recipient{}
	if len(postform.To) > 0 {
		postform.To = append(postform.To, SKeys.Recipient.(*age.X25519Recipient).String())
		for _, to := range postform.To {
			t, err := age.ParseX25519Recipient(to)
			if err != nil {
				continue
			}
			tos = append(tos, t)
		}
	}

	/// --- 3. 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
	log.Println("Attachments start")
	err = uploadFiles(&postform, postMap, tos)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}

	/// --- 4. 对post对象加密（如果不需要加密则保存原始数据）
	data, _ := json.Marshal(postform.post)
	if len(tos) > 0 {
		f := bytes.NewBuffer(data)
		o := bytes.NewBuffer([]byte{})
		err = Encrypt(tos, f, o)
		if err != nil {
			c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("encrypt post err: %s", err.Error())))
			return
		}
		data = o.Bytes()
	}

	postMap[indexFile] = files.NewBytesFile(data)

	/// --- 5. 解析出self发布的最新的cid，并写入到post中的next字段
	log.Println("get last cid start")

	ipfskey, err := getIpfsKey(postform.KeyName)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("get  key err: %s", err.Error())))
		return
	}
	next, err := IpfsAPI.Name().Resolve(context.Background(), ipfskey.Path().String())
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("resolve err: %s", err.Error())))
		return
	}
	postform.Next = next.String()

	/// --- 6. 将meta对象，重新序列化为json，并作为meta.json文件保存
	metaJson, _ := json.Marshal(postform.meta)
	postMap[metaFile] = files.NewBytesFile(metaJson)

	/// --- 7. 将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	log.Println("add to ipfs start")
	cid, err := IpfsAPI.Unixfs().Add(context.Background(), files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("add err: %s", err.Error())))
		return
	}

	/// --- 8. 发布新的cid到self IPNS
	log.Println("publishing name start")
	nameEntry, err := IpfsAPI.Name().Publish(context.Background(), cid, options.Name.Key(ipfskey.Name()))
	if err != nil {
		log.Println("publish error", err)
		c.JSON(http.StatusOK, ResponseJsonFormat(0, fmt.Sprintf("publish err: %s", err.Error())))
		return
	}

	log.Println("name:", nameEntry.Name(), "value:", nameEntry.Value().String())
	//返回结果
	c.JSON(http.StatusOK, ResponseJsonFormat(1, map[string]string{
		"name":  nameEntry.Name(),
		"value": nameEntry.Value().String(),
	}))
}

// 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
func uploadFiles(postform *postForm, postMap map[string]files.Node, tos []age.Recipient) error {
	//迭代所有附件
	//文件名不能为post,也不能重复
	// 如果有to，说明需要加密
	// 使用to里面的公钥加密文件
	// 获得文件加密后的数据（或者不需要加密的原始数据），并且保存到postMap
	for i, u := range postform.Uploads {
		log.Println(i, u.Filename)
		if _, ok := postMap[u.Filename]; u.Filename == indexFile || ok {
			return fmt.Errorf("filename err: %s %t %t", u.Filename, u.Filename == indexFile, ok)
		}

		postform.Attachments = append(postform.Attachments, u.Filename)

		f, err := u.Open()
		if err != nil {
			return fmt.Errorf("open file err: %s", err.Error())
		}

		var data []byte

		if len(postform.To) > 0 {

			o := bytes.NewBuffer([]byte{})
			err = Encrypt(tos, f, o)
			if err != nil {
				return fmt.Errorf("encrypt file err: %s", err.Error())
			}

			data = o.Bytes()
		} else {
			data, err = io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read file err: %s", err.Error())
			}

		}

		postMap[u.Filename] = files.NewBytesFile(data)

		f.Close()
	}
	return nil
}

// 新增IPFS key，也就是新增一个IPNS
func addIpfsKeyHandler(c *gin.Context) {
	// options.NamePublishOption
	name := c.DefaultPostForm("name", "")
	key, err := IpfsAPI.Key().Generate(context.Background(), name)
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, key.Path()))
}

// 列出所有IPFS key，也就是列出所有IPNS
func listIpfsKeyHandler(c *gin.Context) {

	keys, err := IpfsAPI.Key().List(context.Background())
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
	}
	keysArr := [][]string{}
	for _, key := range keys {
		keysArr = append(keysArr, []string{key.Name(), key.Path().String()})
	}

	c.JSON(http.StatusOK, ResponseJsonFormat(1, keysArr))
}

// 使用一个新的密钥，会保留原来的私钥，新增一对公私钥，返回新的公钥
func newSecretKeyHandler(c *gin.Context) {
	var err error
	SKeys, err = NewSecretKey()
	if err != nil {
		c.JSON(http.StatusOK, ResponseJsonFormat(0, err.Error()))
		return
	}
	c.JSON(http.StatusOK, ResponseJsonFormat(1, SKeys.Recipient.(*age.X25519Recipient).String()))
}

// 获得用于加密的公钥
func getSecretKeyHandler(c *gin.Context) {
	c.JSON(http.StatusOK, ResponseJsonFormat(1, SKeys.Recipient.(*age.X25519Recipient).String()))
}

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index", gin.H{})
}

// 格式化返回的JSON，补充将Data的类型，方便前端判断
func ResponseJsonFormat(code int8, data interface{}) responseJson {
	return responseJson{
		Code: code,
		Data: data,
		Type: fmt.Sprintf("%T", data),
	}
}

// 获得IPFS的key对象，如果传递的名称没有对应的key，默认返回self
func getIpfsKey(name string) (icore.Key, error) {
	ipnsKeys, err := IpfsAPI.Key().List(context.Background())
	if err != nil {
		return nil, err
	}

	for _, k := range ipnsKeys {
		if k.Name() == name {
			return k, nil
		}
	}

	return IpfsAPI.Key().Self(context.Background())
}

// json返回值对象
type responseJson struct {
	Code int8        `json:"code"`
	Data interface{} `json:"data"`
	Type string      `json:"type"`
}

// publish的表单request表单对象
type postForm struct {
	post                            //save to post.json
	meta                            //save to meta.json
	Uploads []*multipart.FileHeader `json:"-" form:"uploads"` //upload field
	KeyName string                  `json:"-" form:"keyname"` //which key to publish
}

// 主内容对象
type post struct {
	Body        string   `json:"body" form:"body"`
	Type        string   `json:"type" form:"type"`
	Attachments []string `json:"attachments" form:"-"`
}

// 元数据对象，这个对象中的内容将不加密
type meta struct {
	To   []string `json:"to" form:"to"`
	Next string   `json:"next" form:"next"`
}
