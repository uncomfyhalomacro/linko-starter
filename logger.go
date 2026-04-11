package main

import (
	"log"
	"os"
)

var logger = log.New(os.Stderr, "DEBUG: ", log.LstdFlags)

