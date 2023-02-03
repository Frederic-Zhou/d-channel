package httpapi

import (
	"d-channel/database"
	"net/http"

	"berty.tech/go-orbit-db/iface"
	"github.com/gin-gonic/gin"
)

// 单例，数据库实例
var instance *database.Instance

// 返回消息的的类型
const (
	MSG_SUCCESS = "success"
	MSG_FAIL    = "fail"
	MSG_UNKNOW  = "unknow"
	MSG_ERROR   = "error"
)

// 方法名称，
const (
	METHOD_all    = "all"
	METHOD_put    = "put"
	METHOD_get    = "get"
	METHOD_add    = "add"
	METHOD_delete = "delete"
	METHOD_query  = "query"
)

// 响应的数据结构
type response struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// 运行HTTP接口
func Run(addr string) error {

	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1", "localhost"})

	router.POST("/boot", bootInstance)   // 启动实例
	router.POST("/programs", programs)   // 查看实例内置数据库，其中包含所有数据库信息
	router.POST("/close", closeInstance) //关闭实例
	router.POST("/createdb", createdb)   //创建数据库
	router.POST("/removedb", removedb)   //移除数据库
	router.POST("/closedb", closedb)     //关闭数据库
	router.POST("/command", command)     //执行数据库操作命令

	return router.Run(addr)
}

// 启动实例，运行成功过后，赋值给全局变量 instance
func bootInstance(c *gin.Context) {
	if instance != nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is created"})
		return
	}
	var err error
	instance, err = database.BootInstance(c.Request.Context(), database.DEFAULT_PATH, database.DEFAULT_PATH)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})
}

// 关闭实例，关闭成功后，instance为空
func closeInstance(c *gin.Context) {
	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance nil"})
		return
	}
	err := instance.Close()
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}
	instance = nil
	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})
}

// 创建数据库输入参数
type createIn struct {
	Name      string
	StoreType string
	AccessIDs []string
}

// 创建数据库
func createdb(c *gin.Context) {

	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is null"})
		return
	}

	var err error

	in := &createIn{}
	if err = c.ShouldBind(in); err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	_, err = instance.CreateDB(c.Request.Context(), in.Name, in.StoreType, in.AccessIDs)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})

}

// 数据库命令参数
type commandIn struct {
	Address     string
	Method      string
	Params      params
	OriginPeers []string
}

// 数据库命令参数中的参数
type params struct {
	Key   string
	Value interface{}
}

// 执行数据库命令
func command(c *gin.Context) {

	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is null"})
		return
	}

	var err error
	var db iface.Store

	in := &commandIn{}
	if err = c.ShouldBind(in); err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	//检查是否是连接中的数据库
	db, connecting := instance.ConnectingDB[in.Address]
	//如果不是，连接并添加数据库（添加动作也会覆盖已经保存过的数据库，如果地址相同）
	if !connecting {
		db, err = instance.OpenDB(c.Request.Context(), in.Address, in.OriginPeers)
		if err != nil {
			c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
			return
		}
	}

	//执行数据库操作命令。
	result, err := exec(c.Request.Context(), db, in.Method, in.Params)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS, Data: result})
}

// 获取程序内置数据库，以便于获得其他库的信息。
func programs(c *gin.Context) {
	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance nil"})
		return
	}

	programs, err := instance.GetProgramsDB(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS, Data: programs})
}

type removeIn struct {
	Address string
}

// 删除数据库
func removedb(c *gin.Context) {
	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is null"})
		return
	}

	var err error

	in := &removeIn{}
	if err = c.ShouldBind(in); err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	err = instance.RemoveDB(c.Request.Context(), in.Address)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}
}

type closeIn struct {
	Address string
}

// 关闭数据库
func closedb(c *gin.Context) {
	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is null"})
		return
	}

	var err error

	in := &closeIn{}
	if err = c.ShouldBind(in); err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	err = instance.CloseDB(c.Request.Context(), in.Address)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}
}
