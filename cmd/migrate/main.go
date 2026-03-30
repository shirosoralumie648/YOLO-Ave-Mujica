package main

import (
	"flag"
	"log"
	"os"

	"yolo-ave-mujica/internal/store"
)

func main() {
	command := flag.String("command", "up", "migration command: up|down|version|force")
	source := flag.String("source", "file://migrations", "migration source url")
	forceVersion := flag.Int("force-version", 0, "version used by force")
	flag.Parse()

	if err := store.RunMigrations(os.Getenv("DATABASE_URL"), *source, *command, *forceVersion); err != nil {
		log.Fatal(err)
	}
}
