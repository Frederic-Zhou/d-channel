package httpapi

import (
	"context"
	"d-channel/database"
	"encoding/json"
	"fmt"
	"net/http"

	"berty.tech/go-orbit-db/iface"
	"berty.tech/go-orbit-db/stores/operation"
	"github.com/gin-gonic/gin"
	"github.com/ipfs/go-cid"
)

var instance *database.Instance
var connectingDBs map[string]iface.Store = map[string]iface.Store{}

const (
	MSG_SUCCESS = "success"
	MSG_FAIL    = "fail"
	MSG_UNKNOW  = "unknow"
	MSG_ERROR   = "error"
)

const (
	METHOD_all    = "all"
	METHOD_put    = "put"
	METHOD_get    = "get"
	METHOD_add    = "add"
	METHOD_delete = "delete"
	METHOD_query  = "query"
)

type response struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func Run(addr string) error {

	router := gin.Default()
	router.SetTrustedProxies([]string{"127.0.0.1", "localhost"})

	router.POST("/boot", bootInstance) // 发布
	router.POST("/programs", programs) // 查看实例内所有数据库信息
	router.POST("/close", closeInstance)
	router.POST("/createdb", createdb)
	router.POST("/command", command)

	return router.Run(addr)
}

func bootInstance(c *gin.Context) {
	if instance != nil {
		c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})
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

func closeInstance(c *gin.Context) {
	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance nil"})
		return
	}
	instance.Close()
	instance = nil
	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})
}

type createIn struct {
	Name      string
	StoreType string
	AccessIDs []string
}

func createdb(c *gin.Context) {

	if instance == nil {
		c.JSON(http.StatusOK, response{Message: MSG_FAIL, Data: "instance is null"})
		return
	}

	var err error
	var db iface.Store

	in := &createIn{}
	if err = c.ShouldBind(in); err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	db, err = instance.CreateDB(c.Request.Context(), in.Name, in.StoreType, in.AccessIDs)

	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	connectingDBs[db.Address().String()] = db

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS})

}

type commandIn struct {
	Address string
	Method  string
	Params  params
}

type params struct {
	Key   string
	Value interface{}
}

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
	db, connecting := connectingDBs[in.Address]
	//如果不是，检查是否存储过
	if !connecting {
		db, err = instance.AddDB(c.Request.Context(), in.Address)
		if err != nil {
			c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
			return
		}
		connectingDBs[in.Address] = db
	}

	result, err := exec(c.Request.Context(), db, in.Method, in.Params)
	if err != nil {
		c.JSON(http.StatusOK, response{Message: MSG_ERROR, Data: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{Message: MSG_SUCCESS, Data: result})
}

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

func exec(ctx context.Context, db iface.Store, method string, p params) (any interface{}, err error) {

	switch db.Type() {
	case database.STORETYPE_KV:
		any, err = execKV(ctx, db, method, p)

	case database.STORETYPE_LOG:
		any, err = execLog(ctx, db, method, p)

	case database.STORETYPE_DOCS:
		any, err = execDocs(ctx, db, method, p)

	default:
		err = fmt.Errorf("db type error: %v", db.Type())
	}

	if err != nil {
		return
	}

	if op, ok := any.(operation.Operation); ok {
		var data []byte
		data, err = op.Marshal()
		if err != nil {
			return
		}
		err = json.Unmarshal(data, &any)
	}

	return

}

func execKV(ctx context.Context, db iface.Store, method string, p params) (any interface{}, err error) {
	rdb := db.(iface.KeyValueStore)

	switch method {
	case METHOD_all:
		any = rdb.All()
	case METHOD_put:

		var value []byte
		value, err = json.Marshal(p.Value)
		if err != nil {
			return
		}
		any, err = rdb.Put(ctx, p.Key, value)
	case METHOD_delete:
		any, err = rdb.Delete(ctx, p.Key)
	case METHOD_get:
		any, err = rdb.Get(ctx, p.Key)
	default:
		err = fmt.Errorf("method error: %v", method)
	}

	return

}
func execLog(ctx context.Context, db iface.Store, method string, p params) (any interface{}, err error) {
	rdb := db.(iface.EventLogStore)

	switch method {
	case METHOD_add:
		var value []byte
		value, err = json.Marshal(p.Value)
		if err != nil {
			return
		}
		any, err = rdb.Add(ctx, value)
	case METHOD_get:
		var _cid cid.Cid
		_cid, err = cid.Decode(p.Key)
		if err != nil {
			return
		}
		any, err = rdb.Get(ctx, _cid)
	default:
		err = fmt.Errorf("method error: %v", method)
	}

	return
}

func execDocs(ctx context.Context, db iface.Store, method string, p params) (any interface{}, err error) {
	rdb := db.(iface.DocumentStore)

	switch method {
	case METHOD_put:
		any, err = rdb.Put(ctx, p.Value)
	case METHOD_get:
		any, err = rdb.Get(ctx, p.Key, nil)
	case METHOD_delete:
		any, err = rdb.Delete(ctx, p.Key)
	case METHOD_query:
		// any, err = rdb.Query()
	default:
		err = fmt.Errorf("method error: %v", method)
	}

	return
}
