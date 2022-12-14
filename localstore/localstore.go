package localstore

import (
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const localstorePath = "./localstore.db"

var db *gorm.DB

func InitDB(repo string) {

	var err error
	db, err = gorm.Open(sqlite.Open(filepath.Join(repo, localstorePath)), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(
		&Follow{},
		&Peer{},
		&Message{})

	db.Debug()
}

func WriteMessage(body string) (err error) {
	return db.Create(&Message{Body: body}).Error
}

func AddFollow(name, ns string, isself bool) error {

	if !strings.HasPrefix(ns, "/ipns/") {
		ns = "/ipns/" + ns
	}

	return db.Create(&Follow{Name: name, NS: ns, IsSelf: isself}).Error
}

func UnFollow(id string) error {
	return db.Unscoped().Delete(&Follow{}, id).Error
}

func DelSelfNS(nsName string) error {
	return db.Unscoped().Delete(&Follow{}, "name=? and is_self=1", nsName).Error
}

func AddPeer(name, recipient, peerID string) error {
	return db.Create(&Peer{Name: name, Recipient: recipient, PeerID: peerID}).Error
}
func RemovePeer(id string) error {
	return db.Unscoped().Delete(&Peer{}, id).Error
}

func (f *Follow) Save() error {
	return db.Save(f).Error
}

func (p *Peer) Save() error {
	return db.Save(p).Error
}

func GetFollows(skip, limit int) (follows []Follow, err error) {
	err = db.Where("is_self<>1 or is_self is null").Order("id desc").Offset(skip).Limit(limit).Find(&follows).Error
	return
}

func GetOneFollow(nsValue string) (ns Follow, err error) {
	err = db.Where("ns = ?", nsValue).First(&ns).Error
	return
}

func GetPeers(skip, limit int) (peers []Peer, err error) {
	err = db.Order("id desc").Offset(skip).Limit(limit).Find(&peers).Error
	return
}

func GetMessages(skip, limit int) (messages []Message, err error) {
	err = db.Order("id desc").Offset(skip).Limit(limit).Find(&messages).Error
	return
}

type Message struct {
	gorm.Model
	Body string `json:"body" gorm:"default:''"`
}

type Follow struct {
	gorm.Model
	Name   string `json:"name" gorm:"default:'';unique"`
	NS     string `json:"ns" gorm:"default:'';unique"`
	Latest string `json:"latest"`
	IsSelf bool   `json:"isself" gorm:"default:0"`
}

type Peer struct {
	gorm.Model
	Name      string `json:"name" gorm:"default:'';unique"`
	Recipient string `json:"recipient" gorm:"default:'';unique"`
	PeerID    string `json:"peerid" gorm:"default:'';unique"`
}
