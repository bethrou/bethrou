package main

import "github.com/henrybarreto/bethrou/node/cmd"

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
