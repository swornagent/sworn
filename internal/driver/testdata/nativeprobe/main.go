package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println(`{"type":"thread.started","thread_id":"native-probe"}`)
	for {
		time.Sleep(time.Hour)
	}
}
