package main

import (
	"fmt"
	"flag"
	
	"kubeflux/fluxion"
)




func main () {
	policy := flag.String("policy", "", "Match policy")

	flag.Parse()
	
	fmt.Println("Policy ", policy)

	fc := fluxion.Fluxion{Policy: *policy}

	fc.InitFluxion()

}