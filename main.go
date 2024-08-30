package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

type Task struct {
	Task     string
	Schedule string
}

type Table struct {
	Tasks []Task
}

type Dir struct {
	Path  string
	Label string
}

type Config struct {
	Dirs []Dir
}

func main() {
	// Get existing crontab content
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("error: no crontab file for this user")
		os.Exit(1)
	}

	// Create a tempfile comprised of the existing crontab contents
	tempFile, err := os.CreateTemp(os.TempDir(), "raketab-temp")
	if err != nil {
		log.Fatal(err)
	}
	defer tempFile.Close()

	// Write to tempfile with existing contents
	if err = os.WriteFile(tempFile.Name(), output, fs.ModeAppend); err != nil {
		log.Fatal(err)
	}

	// Get the byte offset of the RAKETABS section of the config file
	scanner := bufio.NewScanner(tempFile)
	offset := 0
	for scanner.Scan() {
		curr := scanner.Text()
		if strings.Contains(curr, "BEGIN RAKETABS") {
			break
		}

		offset += len(scanner.Bytes()) + 1
	}

	// Seek to the offset and truncate anything below it (idempotent)
	_, err = tempFile.Seek(int64(offset), io.SeekStart)
	if err != nil {
		log.Fatal(err)
	}
	err = tempFile.Truncate(int64(offset))
	if err != nil {
		log.Fatal(err)
	}

	// Read config file with all of the repositories and paths and build a map
	// of paths and Table structs
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	config := Config{}
	bytes, err := os.ReadFile(fmt.Sprintf("%v/.raketabs.yml", homeDirectory))
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(bytes, &config)
	if err != nil {
		log.Fatal(err)
	}
	rakeTabs := map[string]Table{}
	for _, tab := range config.Dirs {
		table := Table{}
		bytes, err = os.ReadFile(fmt.Sprintf("%v/%v/.raketab.yml", homeDirectory, tab.Path))
		if err != nil {
			log.Fatal(err)
		}
		err = yaml.Unmarshal(bytes, &table)
		if err != nil {
			log.Fatal(err)
		}
		rakeTabs[tab.Path] = table
	}

	// Build the raketab comment and rake commands and write the temp file
	rakePath, err := exec.LookPath("rake")
	if err != nil {
		log.Fatal("error: couldn't find rake executable")
	}
	content := "# BEGIN RAKETABS GENERATED TASKS - DO NOT EDIT MANUALLY\n"
	for path, task := range rakeTabs {
		for _, task := range task.Tasks {
			content += fmt.Sprintf("%v %v -C %v %v 2>&1| logger -t RAKETAB\n", task.Schedule, rakePath, path, task.Task)
		}
	}
	content += "# END RAKETABS GENERATED TASKS\n"
	_, err = tempFile.Write([]byte(content))
	if err != nil {
		log.Fatal(err)
	}

	// Write the new crontab and remove the tempfile
	cmd = exec.Command("crontab", tempFile.Name())
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	err = os.Remove(tempFile.Name())
	if err != nil {
		log.Fatal(err)
	}
}
