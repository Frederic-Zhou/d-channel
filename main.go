package main

import "d-channel/httpapi"

func main() {

	//TODO: set port from command arguments
	httpapi.Run(":8000")

}
