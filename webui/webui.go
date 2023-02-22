package webui

import (
	"github.com/gin-gonic/gin"
	cors "github.com/rs/cors/wrapper/gin"
)

func Run(addr string) error {

	router := gin.Default()
	router.SetTrustedProxies(nil)
	// router.SetTrustedProxies([]string{"127.0.0.1", "localhost"})
	router.Use(cors.AllowAll()) // 开启 CORS
	//设置静态文件
	router.Static("/asset", "./asset")
	//设置模板文件地址
	router.LoadHTMLGlob("view/*")

	return router.Run(addr)
}
