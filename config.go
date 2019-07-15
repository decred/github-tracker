package main

import (
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
)

type config struct {
	APIToken string `long:"apitoken" description:"github api token"`
	User     string `long:"user" short:"u" description:"github username"`
	Repo     string `long:"repo" short:"r" description:"github repo"`
	Org      string `long:"org" short:"o" description:"github organization"`
	PRNum    int    `long:"prnum" description:"github pr number"`
	Update   bool   `long:"update" description:"fetch latest github data"`
}

func loadConfig() *config {
	var cfg config

	parser := flags.NewParser(&cfg, flags.HelpFlag)
	_, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(0)
		}
	}

	if cfg.Org == "" {
		fmt.Fprintln(os.Stderr, "please specify the organization")
		os.Exit(1)
	}

	if cfg.PRNum != 0 && cfg.Repo == "" {
		fmt.Fprintln(os.Stderr, "prnum requires specifying the repository")
		os.Exit(1)
	}

	return &cfg
}
