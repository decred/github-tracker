package main

import (
	"fmt"
	"os"

	flags "github.com/jessevdk/go-flags"
)

const (
	defaultAPIToken = "975b07d598ac18456f6657e6a6cdebbdd1d9ca35"
)

type config struct {
	APIToken string `long:"apitoken" description:"github api token"`
	User     string `long:"user" short:"u" description:"github username"`
	Repo     string `long:"repo" short:"r" description:"github repo"`
	Org      string `long:"org" short:"o" description:"github organization"`
	PRNum    int    `long:"prnum" description:"github pr number"`
	Update   bool   `long:"update" description:"fetch latest github data"`
	Month    int    `long:"month" description:"specific month"`
	Year     int    `long:"year" description:"specific year"`
}

func loadConfig() *config {
	cfg := config{
		APIToken: defaultAPIToken,
	}

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
