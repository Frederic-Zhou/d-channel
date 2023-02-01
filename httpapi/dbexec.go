package httpapi

import (
	"context"
	"d-channel/database"
	"encoding/json"
	"fmt"

	"berty.tech/go-orbit-db/iface"
	"berty.tech/go-orbit-db/stores/operation"
	"github.com/ipfs/go-cid"
)

// 执行数据库命令
func exec(ctx context.Context, db iface.Store, method string, p params) (any interface{}, err error) {

	//根据数据库类型字符串判断，进入不同的数据库命令函数
	switch db.Type() {
	case database.STORETYPE_KV: //KV 数据库
		any, err = execKV(ctx, db, method, p)
	case database.STORETYPE_LOG: //LOG 数据库
		any, err = execLog(ctx, db, method, p)
	case database.STORETYPE_DOCS: //DOCS 数据库
		any, err = execDocs(ctx, db, method, p)
	default: //如果都不是，返回错误
		err = fmt.Errorf("db type error: %v", db.Type())
	}

	if err != nil {
		return
	}

	//当返回的类型是operation.Operation，拿到any序列化后的Json字符串，然后填充成Map[string]interface{}
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
		value, err = json.Marshal(p.Value) //参数value都解析成Json字符串数组
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
		any, err = rdb.Query(ctx, func(doc interface{}) (bool, error) {
			entity, ok := doc.(map[string]interface{})
			if !ok {
				return false, nil
			}
			if entity[p.Key] == p.Value {
				return true, nil
			}
			return false, nil
		})
	default:
		err = fmt.Errorf("method error: %v", method)
	}

	return
}
