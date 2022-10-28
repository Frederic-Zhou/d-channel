package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"

	"filippo.io/age"
	"github.com/gin-gonic/gin"
	files "github.com/ipfs/go-ipfs-files"
	icore "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
)

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode
var SecretKeys []Key

const indexFile = "post.json"
const metaFile = "meta.json"

func StartWeb(ipfsAPI icore.CoreAPI, ipfsNode *core.IpfsNode, secretkeys []Key) {
	IpfsAPI = ipfsAPI
	IpfsNode = ipfsNode
	SecretKeys = secretkeys

	router := gin.Default()

	router.GET("/ipns/:name/*path", ipnsHandler)
	router.GET("/ipfs/:cid/*path", ipfsHandler)
	router.POST("/publish", publishHandler)
	router.GET("/index", indexHandler)

	router.Run(":8088")
}

func ipnsHandler(c *gin.Context) {
	name := c.Param("name")
	fpath := c.Param("path")

	log.Println("fullpath ", name+fpath)

	fullPath, err := IpfsAPI.Name().Resolve(context.Background(), name+fpath)

	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("err %s", err.Error())})
		return
	}

	//pin
	err = IpfsAPI.Pin().Add(context.Background(), fullPath)

	log.Println("path:", fullPath.String(), "pin err:", err)

	c.Redirect(http.StatusTemporaryRedirect, fullPath.String())

}

func ipfsHandler(c *gin.Context) {
	cid := c.Param("cid")
	fpath := c.Param("path")

	nd, err := IpfsAPI.Unixfs().Get(context.Background(), path.New(cid+fpath))
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("err %s", err.Error())})
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
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: "not a file"})
		return
	}

	//在这里解密
	identitys := []age.Identity{}
	for _, sk := range SecretKeys {
		identitys = append(identitys, sk.Identity)
	}

	o := bytes.NewBuffer([]byte{})
	err = Decrypt(identitys, f, o)
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("decrypt err:%s", err.Error())})
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
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("bind err: %s", err.Error())})
		return
	}

	/// --- 2. 获得加密Pubkeys
	tos := []age.Recipient{}
	for _, to := range postform.To {
		t, err := age.ParseX25519Recipient(to)
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("to err: %s", err.Error())})
			return
		}
		tos = append(tos, t)
	}
	//如果tos不是0 ，那么加上自己的 secretkey ,否则自己是看不到自己发的消息的
	if len(tos) != 0 {
		for _, sk := range SecretKeys {
			tos = append(tos, sk.Recipient)
		}
	}

	/// --- 3. 从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
	log.Println("Attachments start")

	for i, u := range postform.Uploads {
		log.Println(i, u.Filename)
		if _, ok := postMap[u.Filename]; u.Filename == indexFile || ok { //文件名不能为post,也不能重复
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("filename err: %s %t %t", u.Filename, u.Filename == indexFile, ok)})
			return
		}

		postform.Attachments = append(postform.Attachments, u.Filename)

		f, err := u.Open()
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("open file err: %s", err.Error())})
			return
		}

		//使用to里面的公钥加密文件
		o := bytes.NewBuffer([]byte{})
		err = Encrypt(tos, f, o, true)
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("encrypt err: %s", err.Error())})
			return
		}

		//获得文件的files.Node对象，并且保存到postMap
		postMap[u.Filename] = files.NewBytesFile(o.Bytes())
		f.Close()
	}

	/// --- 4. 对post对象加密
	postJson, _ := json.Marshal(postform.post)
	f := bytes.NewBuffer(postJson)
	o := bytes.NewBuffer([]byte{})
	err = Encrypt(tos, f, o, true)
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("encrypt err: %s", err.Error())})
		return
	}

	postMap[indexFile] = files.NewBytesFile(o.Bytes())

	/// --- 5. 解析出self发布的最新的cid，并写入到post中的next字段
	log.Println("get Self key start")
	self, err := IpfsAPI.Key().Self(context.Background())
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("get self key err: %s", err.Error())})
		return
	}
	log.Println("get last cid start")
	next, err := IpfsAPI.Name().Resolve(context.Background(), self.Path().String())
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("resolve err: %s", err.Error())})
		return
	}
	postform.Next = next.String()

	/// --- 6. 将meta对象，重新序列化为json，并作为post.json文件
	metaJson, _ := json.Marshal(postform.meta)
	postMap[metaFile] = files.NewBytesFile(metaJson)

	/// --- 7. 将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	log.Println("add to ipfs start")
	cid, err := IpfsAPI.Unixfs().Add(context.Background(), files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("add err: %s", err.Error())})
		return
	}

	/// --- 8. 发布新的cid到self IPNS
	log.Println("publishing name start")
	nameEntry, err := IpfsAPI.Name().Publish(context.Background(), cid)
	if err != nil {
		log.Println("publish error", err)
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("publish err: %s", err.Error())})
		return
	}

	log.Println("name:", nameEntry.Name(), "value:", nameEntry.Value().String())
	//返回结果
	c.JSON(http.StatusOK, responseJson{Code: 1, Data: map[string]string{
		"name":  nameEntry.Name(),
		"value": nameEntry.Value().String(),
	}})
}

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index", gin.H{})
}

type responseJson struct {
	Code int8        `json:"code"`
	Data interface{} `json:"data"`
}

type postForm struct {
	post
	meta
	Uploads []*multipart.FileHeader `json:"-" form:"uploads"`
}
type post struct {
	Body        string   `json:"body" form:"body"`
	Type        string   `json:"type" form:"type"`
	Attachments []string `json:"attachments" form:"-"`
}

type meta struct {
	To   []string `json:"to" form:"to"`
	Next string   `json:"next" form:"next"`
}
