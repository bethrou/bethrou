package main

import "github.com/henrybarreto/bethrou/client/cmd"

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
