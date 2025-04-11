package main

import (
	"html/template"
	"log"
	"os"
	"os/exec"
	"runtime"
)

const htmlFileName = "connect-links.html"

func openInBrowser(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", path)
	}

	return cmd.Start()
}

type Server struct {
	Name string
	Port int
}

type PageData struct {
	Servers []Server
}

func outputServerList() {
	if !outputHTML {
		return
	}

	data := PageData{
		Servers: []Server{
			{"Map A", 20000},
			{"Map B", 20001},
			{"Map C", 20002},
			{"Map D", 20003},
			{"Map E", 20004},
			{"Map F", 20005},
			{"Map G", 20006},
			{"Map H", 20007},
			{"Map I", 20008},
			{"Map J", 20009},
			{"Map K", 20010},
			{"Map L", 20011},
			{"Map M", 20012},
			{"Map N", 20013},
			{"Map O", 20014},
			{"Map P", 20015},
			{"Map Q", 20016},
			{"Map R", 20017},
		},
	}

	tmpl, err := template.New("page").Parse(pageTemplate)
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	f, err := os.Create(htmlFileName)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer f.Close()

	err = tmpl.Execute(f, data)
	if err != nil {
		log.Fatalf("Failed to execute template: %v", err)
	}

	log.Printf("UPDATED %v written successfully... (attempting to open)", htmlFileName)

	if err := openInBrowser(htmlFileName); err != nil {
		log.Printf("Failed to open in browser: %v", err)
	} else {
		log.Printf("The server list should now be open!")
	}
}
