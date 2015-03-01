package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"

	"github.com/zachlatta/orbit"
	"gopkg.in/fsnotify.v1"
)

var (
	baseURLStr = flag.String("url", "http://mew.hackedu.us:4000", "base URL of orbit")
	baseURL    *url.URL

	apiClient = orbit.NewClient(nil)
)

func init() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `orbit puts your development environment in the cloud.

Usage:

    orbit [options] command [arg...]

The commands are:
`)
		for _, c := range subcmds {
			fmt.Fprintf(os.Stderr, "    %-24s %s\n", c.name, c.description)
		}
		fmt.Fprintln(os.Stderr, `
Use "orbit command -h" for more information about a command.

The options are:
`)
		flag.PrintDefaults()
		os.Exit(1)
	}
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
	}
	log.SetFlags(0)

	var err error
	baseURL, err = url.Parse(*baseURLStr)
	if err != nil {
		log.Fatal(err)
	}
	apiClient.BaseURL = baseURL.ResolveReference(&url.URL{Path: "/api/"})

	subcmd := flag.Arg(0)
	for _, c := range subcmds {
		if c.name == subcmd {
			c.run(flag.Args()[1:])
			return
		}
	}

	fmt.Fprintf(os.Stderr, "unknown subcmd %q\n", subcmd)
	fmt.Fprintln(os.Stderr, `Run "orbit -h" for usage.`)
	os.Exit(1)
}

type subcmd struct {
	name        string
	description string
	run         func(args []string)
}

var subcmds = []subcmd{
	{"daemon", "start the orbit daemon", daemonCmd},
	{"create-project", "create a new project", createProjectCmd},
}

func daemonCmd(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: orbit daemon [options]

Start the Orbit daemon that watches for and acts on file changes.

The options are:
`)
		fs.PrintDefaults()
		os.Exit(1)
	}
	fs.Parse(args)

	if fs.NArg() != 0 {
		fs.Usage()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case e := <-watcher.Events:
				if e.Op&fsnotify.Write == fsnotify.Write {
					if err := commitAndPushEverything(); err != nil {
						log.Fatal("error committing changes")
					}
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(".")
	if err != nil {
		log.Fatal(err)
	}
	<-done
}

func commitAndPushEverything() error {
	if err := exec.Command("git", "add", "-A", ":/").Run(); err != nil {
		return err
	}

	if err := exec.Command("git", "commit", "-m", "", "--allow-empty-message", "--allow-empty").Run(); err != nil {
		return err
	}

	if err := exec.Command("git", "push").Run(); err != nil {
		return err
	}

	return nil
}

func createProjectCmd(args []string) {
	fs := flag.NewFlagSet("create-project", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: orbit create-project [project name] [options]

Create a new project on Orbit.
`)
		os.Exit(1)
	}
	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
	}

	projectName := fs.Args()[0]

	var project orbit.Project
	if err := apiClient.Projects.Create(&project); err != nil {
		log.Fatal(err)
	}
	cloneURL := baseURL.ResolveReference(&url.URL{Path: "/git/"}).ResolveReference(&url.URL{Path: project.GitPath})
	if err := exec.Command("git", "clone", cloneURL.String(), projectName).Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s created successfully\n", projectName)
}
