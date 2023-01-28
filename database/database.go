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

	// config "github.com/ipfs/go-ipfs-config"

	icore "github.com/ipfs/interface-go-ipfs-core"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/crypto"
)

const (
	DEFAULT_PATH = "dafault"
	PROGRAMSDB   = "self.programs"
)

type Instance struct {
	ctx context.Context

	Dir  string //orbitdb dirctory
	Repo string

	IPFSNode    *core.IpfsNode
	IPFSCoreAPI icore.CoreAPI

	OrbitDB  orbitdb.OrbitDB
	Programs orbitdb.KeyValueStore
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

		dbpath = filepath.Join(dbpath, "orbitdb")
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

	_, err = ins.GetProgramsDB()

	return
}

func (ins *Instance) CreateDB(name, storetype string, accesseIDs []string) (db iface.Store, err error) {

	if name == PROGRAMSDB {
		err = fmt.Errorf("name can not be '%s'", PROGRAMSDB)
		return
	}
	ac := &accesscontroller.CreateAccessControllerOptions{
		Access: map[string][]string{
			"write": accesseIDs,
		},
	}

	db, err = ins.OrbitDB.Create(ins.ctx, name, storetype, &orbitdb.CreateDBOptions{
		AccessController: ac,
	})
	if err != nil {
		return
	}

	dbinfo, err := json.Marshal(DBInfo{
		Name:    name,
		Type:    storetype,
		Address: db.Address().String(),
		AddedAt: time.Now().String(),
	})
	if err != nil {
		return
	}

	_, err = ins.Programs.Put(ins.ctx, db.Address().String(), dbinfo)

	return
}

func (ins *Instance) GetDB(address string) (db iface.Store, err error) {

	db, err = ins.OrbitDB.Open(ins.ctx, address, &orbitdb.CreateDBOptions{})
	if err != nil {
		return
	}
	err = db.Load(ins.ctx, -1)
	return
}

func (ins *Instance) AddDB(address string) (db iface.Store, err error) {

	db, err = ins.GetDB(address)
	if err != nil {
		return
	}
	_, err = ins.Programs.Put(ins.ctx, address, []byte{})

	return
}

func (ins *Instance) RemoveDB(address string) (err error) {
	_, err = ins.Programs.Delete(ins.ctx, address)
	return
}

func (ins *Instance) GetOwnID() string {
	return ins.OrbitDB.Identity().ID
}

func (ins *Instance) GetOwnPubKey() crypto.PubKey {
	pubKey, err := ins.OrbitDB.Identity().GetPublicKey()
	if err != nil {
		return nil
	}

	return pubKey
}

func (ins *Instance) Close() {
	ins.OrbitDB.Close()
}

func (ins *Instance) GetProgramsDB() (program map[string][]byte, err error) {
	if ins.Programs == nil && ins.OrbitDB != nil {
		ins.Programs, err = ins.OrbitDB.KeyValue(ins.ctx, PROGRAMSDB, &orbitdb.CreateDBOptions{})
		if err != nil {
			return
		}
		err = ins.Programs.Load(ins.ctx, -1)
		if err != nil {
			return
		}
	}

	return ins.Programs.All(), nil
}

type DBInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address"`
	AddedAt string `json:"addat"`
}