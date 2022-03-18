package main

import (
	"fmt"

	"./strategy"
)

func main() {

	fmt.Println("CURRENT STRATEGY [BOLLINGER]")
	strategy.BollingerStart()
}
