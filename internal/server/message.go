package server

import (
	"errors"
	"strconv"
	"strings"
)

type Message struct {
	Command byte
	Slot    int
	Value   string
}

var SupportedCommands = map[string]bool{
	"r": true,
	"w": true,
	"u": true,
	"p": true,
}

func ParseMessage(size int, buf []byte) (*Message, error) {
	if buf[size-1] != '\n' {
		return nil, errors.New("Message is malformed")
	}
	input := strings.Split(string(buf), "\n")[0]

	if len(input) < 4 {
		return nil, errors.New("Message is too short")
	}

	if len(input) > 40 {
		return nil, errors.New("Message is too long")
	}

	command := input[:1]

	if SupportedCommands[command] != true {
		return nil, errors.New("Command not supported")
	}

	if command == "u" || command == "p" {
		return &Message{Command: []byte(command)[0], Slot: 0, Value: input[1:]}, nil
	}

	slot, err := strconv.Atoi(input[1:4])
	if err != nil {
		return nil, errors.New("Malformed slot")
	}

	var value string
	if command == "w" {
		value = input[4:]
	}

	return &Message{Command: []byte(command)[0], Slot: slot, Value: value}, nil
}
