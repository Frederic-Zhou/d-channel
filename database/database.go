package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	orbitdb "berty.tech/go-orbit-db"
	"berty.tech/go-orbit-db/accesscontroller"
	"berty.tech/go-orbit-db/iface"
	"berty.tech/go-orbit-db/stores"

	icore "github.com/ipfs/interface-go-ipfs-core"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	DEFAULT_PATH = "dafault"
	PROGRAMSDB   = "self.programs"
	ORBITDIR     = "orbitdb"

	STORETYPE_KV   = "keyvalue"
	STORETYPE_DOCS = "docstore"
	STORETYPE_LOG  = "eventlog"
)

type Instance struct {
	lifecircle_ctx context.Context
	Dir            string //orbitdb dirctory
	Repo           string //ipfs repo path

	IPFSNode    *core.IpfsNode //ipfsnode
	IPFSCoreAPI icore.CoreAPI  //ipfscoreapi

	OrbitDB  orbitdb.OrbitDB       //orbitdb object
	Programs orbitdb.KeyValueStore // buildin db, local-only, to store other dbs information

	ConnectingDB map[string]iface.Store
}
type DBInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Address string   `json:"address"`
	AddedAt string   `json:"addat"`
	Peers   []string `json:"peers"`
}

// BootInstance 启动一个实例
func BootInstance(ctx context.Context, repoPath, dbpath string) (ins *Instance, err error) {

	ins = new(Instance)
	ins.ConnectingDB = map[string]iface.Store{}
	ins.lifecircle_ctx = ctx

	if repoPath == DEFAULT_PATH {
		repoPath, err = config.PathRoot()
		if err != nil {
			return
		}
	}

	if dbpath == DEFAULT_PATH {
		dbpath, err = os.UserHomeDir()
		if err != nil {
			return
		}
		dbpath = filepath.Join(dbpath, ORBITDIR)
	}

	ins.Dir = dbpath
	ins.Repo = repoPath

	if err = setupPlugins(repoPath); err != nil {
		//TODO: 为什么第二次启动会报错。
		// return
	}

	ins.IPFSNode, ins.IPFSCoreAPI, err = createNode(ctx, repoPath)
	if err != nil {
		return
	}

	ins.OrbitDB, err = orbitdb.NewOrbitDB(ctx, ins.IPFSCoreAPI, &orbitdb.NewOrbitDBOptions{
		Directory: &ins.Dir,
	})
	if err != nil {
		return
	}

	_, err = ins.GetProgramsDB(ctx)

	return
}

func (ins *Instance) CreateDB(ctx context.Context, name string, storetype string, accesseIDs []string) (db iface.Store, err error) {

	if name == PROGRAMSDB {
		err = fmt.Errorf("name can not be '%s'", PROGRAMSDB)
		return
	}
	ac := &accesscontroller.CreateAccessControllerOptions{
		Access: map[string][]string{
			"write": accesseIDs,
		},
	}

	db, err = ins.OrbitDB.Create(ctx, name, storetype, &orbitdb.CreateDBOptions{
		AccessController: ac,
	})
	if err != nil {
		return
	}

	err = ins.initDB(ctx, db, []string{})
	if err != nil {
		return
	}

	return
}

func (ins *Instance) OpenDB(ctx context.Context, address string, originPeers []string) (db iface.Store, err error) {

	db, err = ins.OrbitDB.Open(ctx, address, &orbitdb.CreateDBOptions{})
	if err != nil {
		return
	}

	err = db.Load(ctx, -1)
	if err != nil {
		return
	}

	err = ins.initDB(ctx, db, originPeers)
	if err != nil {
		return
	}

	return
}

func (ins *Instance) RemoveDB(ctx context.Context, address string) (err error) {

	db, ok := ins.ConnectingDB[address]
	if ok {
		err = db.Drop()
		if err != nil {
			return
		}

		err = db.Close()
		if err != nil {
			return
		}
		delete(ins.ConnectingDB, address)

	}

	_, err = ins.Programs.Delete(ctx, address)
	return
}

func (ins *Instance) CloseDB(ctx context.Context, address string) (err error) {
	db, ok := ins.ConnectingDB[address]
	if ok {
		err = db.Close()
		if err != nil {
			return
		}
		delete(ins.ConnectingDB, address)
	}

	return
}

func (ins *Instance) Close() (err error) {
	if err = ins.Programs.Close(); err != nil {
		return
	}

	for _, db := range ins.ConnectingDB {
		if err = db.Close(); err != nil {
			return
		}
	}

	ins.ConnectingDB = map[string]iface.Store{}

	if err = ins.OrbitDB.Close(); err != nil {
		return
	}
	ins = nil
	return
}

// func (ins *Instance) GetOwnID() string {
// 	return ins.OrbitDB.Identity().ID
// }

// func (ins *Instance) GetOwnPubKey() (pubKey crypto.PubKey, err error) {
// 	return ins.OrbitDB.Identity().GetPublicKey()
// }

func (ins *Instance) GetProgramsDB(ctx context.Context) (program map[string][]byte, err error) {
	localonly := true //programs 不在网络同步
	if ins.Programs == nil && ins.OrbitDB != nil {
		ins.Programs, err = ins.OrbitDB.KeyValue(ctx, PROGRAMSDB, &orbitdb.CreateDBOptions{
			LocalOnly: &localonly,
		})
		if err != nil {
			return
		}
		err = ins.Programs.Load(ctx, -1)
		if err != nil {
			return
		}
	}

	return ins.Programs.All(), nil
}

func (ins *Instance) initDB(ctx context.Context, db iface.Store, originPeers []string) (err error) {

	//如果没有保存在program，就保存进去
	var value []byte
	value, err = ins.Programs.Get(ctx, db.Address().String())
	if err != nil || value == nil {
		value, err = json.Marshal(DBInfo{
			Name:    db.DBName(),
			Type:    db.Type(),
			Address: db.Address().String(),
			AddedAt: time.Now().String(),
			Peers:   originPeers,
		})
		if err != nil {
			return err
		}
		_, err = ins.Programs.Put(ctx, db.Address().String(), value)
		if err != nil {
			return
		}
	}

	//如果没有在已连接数据库里，就连接，并监听
	_, ok := ins.ConnectingDB[db.Address().String()]
	if !ok {
		ins.ConnectingDB[db.Address().String()] = db
		go ins.listenPeerEvent(db, value)
	}

	return
}

func (ins *Instance) listenPeerEvent(db iface.Store, value []byte) {
	var err error
	var newPeerEvent event.Subscription
	newPeerEvent, err = db.EventBus().Subscribe(new(stores.EventNewPeer))
	if err != nil {
		return
	}

	dbinfo := DBInfo{}
	err = json.Unmarshal(value, &dbinfo)
	if err != nil {
		return
	}

	//尝试连接所有peer
	go func(ctx context.Context, peers []string) {
		for {
			select {
			case <-time.After(30 * time.Second):
				for _, p := range peers {

					peerid, err := peer.Decode(p)
					if err != nil {
						continue
					}
					peerAddresinfo, err := ins.IPFSCoreAPI.Dht().FindPeer(ctx, peerid)
					if err != nil {
						continue
					}
					_ = ins.IPFSCoreAPI.Swarm().Connect(ctx, peerAddresinfo)

				}
			case <-ctx.Done():
				return
			}
		}
	}(ins.lifecircle_ctx, dbinfo.Peers)

	for {
		select {
		case ev := <-newPeerEvent.Out():

			newPeerID := ev.(stores.EventNewPeer).Peer.String()

			dbinfo.Peers = append(dbinfo.Peers, newPeerID)

			value, err = json.Marshal(dbinfo)
			if err != nil {
				continue
			}

			ins.Programs.Put(ins.lifecircle_ctx, db.Address().String(), value)

		case <-ins.lifecircle_ctx.Done():
		}
	}
}
