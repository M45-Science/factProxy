package main

import (
	"html/template"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var outputHTML, forceHTML bool

const htmlFileName = "connect-links.html"

type ServerEntry struct {
	Name string
	Port int
}

type PageData struct {
	Servers []ServerEntry
}

func outputServerList() {

	if !outputHTML && !forceHTML {
		return
	}

	data := PageData{Servers: []ServerEntry{}}

	for i, l := range gamePorts {
		server := ServerEntry{Name: intToLabel(i), Port: l}
		data.Servers = append(data.Servers, server)
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

// intToLabel converts an integer to a string like "a".."zzz"
func intToLabel(n int) string {
	if n < 0 {
		return ""
	}
	result := ""
	for {
		rem := n % 26
		result = string('a'+rem) + result
		n = n/26 - 1
		if n < 0 {
			break
		}
	}
	return strings.ToUpper(result)
}
