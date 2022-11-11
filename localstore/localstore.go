package localstore

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const localstorePath = "./localstore.db"

var db *gorm.DB

func InitDB() {

	var err error
	db, err = gorm.Open(sqlite.Open(localstorePath), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(&Follow{}, &Peer{}, &Message{})
	db.Debug()
}

func WriteMessage(body string) (err error) {
	return db.Create(&Message{Body: body}).Error
}

func AddFollow(name, addr string) error {
	return db.Create(&Follow{Name: name, Addr: addr}).Error
}

func UnFollow(id string) error {
	return db.Delete(&Follow{}, id).Error
}

func AddPeer(name, recipient, pubkey, peerID string) error {
	return db.Create(&Peer{Name: name, Recipient: recipient, PubKey: pubkey, PeerID: peerID}).Error
}
func RemovePeer(id string) error {
	return db.Delete(&Peer{}, id).Error
}

func (f *Follow) Save() error {
	return db.Save(f).Error
}

func (p *Peer) Save() error {
	return db.Save(p).Error
}

func GetFollows(skip, limit int) (follows []Follow, err error) {

	err = db.Order("id desc").Offset(skip).Limit(limit).Find(&follows).Error
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
	Addr   string `json:"addr" gorm:"default:'';unique"`
	Latest string `json:"latest"`
}

type Peer struct {
	gorm.Model
	Name      string `json:"name" gorm:"default:'';unique"`
	Recipient string `json:"recipient" gorm:"default:'';unique"`
	PubKey    string `json:"pubkey" gorm:"default:'';unique"`
	PeerID    string `json:"peerid" gorm:"default:'';unique"`
}
