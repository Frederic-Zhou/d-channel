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

	icore "github.com/ipfs/interface-go-ipfs-core"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
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
	Dir  string //orbitdb dirctory
	Repo string //ipfs repo path

	IPFSNode    *core.IpfsNode //ipfsnode
	IPFSCoreAPI icore.CoreAPI  //ipfscoreapi

	OrbitDB  orbitdb.OrbitDB       //orbitdb object
	Programs orbitdb.KeyValueStore // buildin db, local-only, to store other dbs information
}
type DBInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	AddedAt string `json:"addat"`
}

// BootInstance 启动一个实例
func BootInstance(ctx context.Context, repoPath, dbpath string) (ins *Instance, err error) {

	ins = new(Instance)

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
		return
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

	return
}

func (ins *Instance) CreateDB(ctx context.Context, name string, storetype string, accesseIDs []string) (db iface.Store, events event.Subscription, err error) {

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

	events, err = db.EventBus().Subscribe(event.WildcardSubscription)
	if err != nil {
		return
	}

	dbinfo, err := json.Marshal(DBInfo{
		Name:    db.DBName(),
		Type:    db.Type(),
		Address: db.Address().String(),
		AddedAt: time.Now().String(),
	})
	if err != nil {
		return
	}

	_, err = ins.Programs.Put(ctx, db.Address().String(), dbinfo)

	return
}

func (ins *Instance) OpenDB(ctx context.Context, address string) (db iface.Store, events event.Subscription, err error) {

	db, err = ins.OrbitDB.Open(ctx, address, &orbitdb.CreateDBOptions{})
	if err != nil {
		return
	}

	err = db.Load(ctx, -1)
	if err != nil {
		return
	}

	events, err = db.EventBus().Subscribe(event.WildcardSubscription)

	return
}

func (ins *Instance) AddDB(ctx context.Context, address string) (db iface.Store, events event.Subscription, err error) {

	db, events, err = ins.OpenDB(ctx, address)
	if err != nil {
		return
	}

	dbinfo, err := json.Marshal(DBInfo{
		Name:    db.DBName(),
		Type:    db.Type(),
		Address: db.Address().String(),
		AddedAt: time.Now().String(),
	})
	if err != nil {
		return
	}

	_, err = ins.Programs.Put(ctx, db.Address().String(), dbinfo)

	return
}

func (ins *Instance) RemoveDB(ctx context.Context, address string) (err error) {
	_, err = ins.Programs.Delete(ctx, address)
	return
}

func (ins *Instance) GetOwnID() string {
	return ins.OrbitDB.Identity().ID
}

func (ins *Instance) GetOwnPubKey() (pubKey crypto.PubKey, err error) {
	return ins.OrbitDB.Identity().GetPublicKey()
}

func (ins *Instance) Close() (err error) {
	if err = ins.Programs.Close(); err != nil {
		return
	}

	if err = ins.OrbitDB.Close(); err != nil {
		return
	}
	return
}

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

func TestDB() {
	// _ = iface.KeyValueStore{}
	// _ = iface.DocumentStore{}
	// _ = iface.EventLogStore{}

	// iface.Store

}
