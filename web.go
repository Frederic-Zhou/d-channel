package main

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
)

var IpfsAPI icore.CoreAPI
var IpfsNode *core.IpfsNode

const indexFile = "post.json"

func StartWeb(ipfsAPI icore.CoreAPI, ipfsNode *core.IpfsNode) {
	IpfsAPI = ipfsAPI
	IpfsNode = ipfsNode

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
		c.String(http.StatusOK, "err %s", err.Error())
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

	fullPath := cid + fpath
	log.Println("fullPath ", fullPath)

	nd, err := IpfsAPI.Unixfs().Get(context.Background(), path.New(fullPath))
	if err != nil {
		c.String(http.StatusOK, "err %s", err)
		return
	}
	defer nd.Close()

	fs := []string{}
	files.Walk(nd, func(fpath string, nd files.Node) error {
		fs = append(fs, fpath)
		return nil
	})

	//如果是单文件，就以文件处理，如果多文件，就认为是目录，列出所有文件
	if len(fs) == 1 {
		f := files.ToFile(nd)
		if f == nil {
			c.String(http.StatusOK, "not a file")
			return
		}

		data, err := io.ReadAll(f)
		if err != nil {
			c.String(http.StatusOK, "read data err %s", err.Error())
			return
		}

		//在这里解密
		//读取指定的公钥文件

		if len(data) == 0 {
			c.String(http.StatusOK, "data is empty")
			return
		}

		c.Data(http.StatusOK, http.DetectContentType(data), data)
		return
	}

	c.JSON(http.StatusOK, fs)

}

func publishHandler(c *gin.Context) {
	log.Println("publish start")
	//定义一个files.Nodes map,用于上传到IPFS网络
	postMap := map[string]files.Node{}

	//从请求中提取出请求内容
	var post postJson
	err := c.ShouldBind(&post)
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("bind err: %s", err.Error())})
		return
	}

	//获得加密Pubkeys
	tos := []age.Recipient{}
	for _, to := range post.To {
		t, err := age.ParseX25519Recipient(to)
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("to err: %s", err.Error())})
			return
		}
		tos = append(tos, t)
	}

	//从请求内容中，提取出需要上传的文件，并写入到 postMap, 修改post中附件文件路径为文件名
	log.Println("Attachments start")

	for i, u := range post.Uploads {
		log.Println(i, u.Filename)
		if _, ok := postMap[u.Filename]; u.Filename == indexFile || ok { //文件名不能为post,也不能重复
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("filename err: %s %t %t", u.Filename, u.Filename == indexFile, ok)})
			return
		}

		post.Attachments = append(post.Attachments, u.Filename)

		f, err := u.Open()
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("open file err: %s", err.Error())})
			return
		}
		defer f.Close()

		//使用to里面的公钥加密文件
		var dst bytes.Buffer
		w, err := age.Encrypt(&dst, tos...)
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("encrypt err: %s", err.Error())})
			return
		}
		defer w.Close()

		_, err = io.Copy(w, f)
		if err != nil {
			c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("io.copy err: %s", err.Error())})
			return
		}
		//获得文件的files.Node对象，并且保存到postMap
		postMap[u.Filename] = files.NewBytesFile(dst.Bytes())
	}

	//解析出self key下发布的最新的cid，并写入到post中的next字段
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
	post.Next = next.String()

	//对Body加密
	var dst bytes.Buffer
	w, err := age.Encrypt(&dst, tos...)
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("body encrypt err: %s", err.Error())})
		return
	}
	if _, err = io.WriteString(w, post.Body); err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("body write err: %s", err.Error())})
		return
	}
	defer w.Close()

	post.Body = base64.RawStdEncoding.EncodeToString(dst.Bytes())

	//将post对象，重新序列化为json，并作为post.json文件
	postBody, _ := json.Marshal(post)
	postMap[indexFile] = files.NewBytesFile(postBody)

	//将整个 postMap（包含post.json和所有附件）， 添加到IPFS网络,获得cid
	log.Println("add to ipfs start")
	cid, err := IpfsAPI.Unixfs().Add(context.Background(), files.NewMapDirectory(postMap))
	if err != nil {
		c.JSON(http.StatusOK, responseJson{Code: 0, Data: fmt.Sprintf("add err: %s", err.Error())})
		return
	}

	//发布新的cid到self key
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

type postJson struct {
	Subject     string                  `json:"subject" form:"subject"`
	Body        string                  `json:"body" form:"body"`
	Type        string                  `json:"type" form:"type"`
	To          []string                `json:"to" form:"to"`
	Uploads     []*multipart.FileHeader `json:"-" form:"uploads"`
	Attachments []string                `json:"attachments" form:"-"`
	Next        string                  `json:"next" form:"next"`
}
