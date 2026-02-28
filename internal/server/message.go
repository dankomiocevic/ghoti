package server

import (
	"errors"
	"fmt"
	"strconv"
)

type Message struct {
	Command byte
	Slot    int
	Value   string
	Raw     string
}

var SupportedCommands = map[string]bool{
	"r": true,
	"w": true,
	"u": true,
	"p": true,
	"j": true,
	"q": true,
}

func ParseMessage(size int, buf []byte) (Message, error) {
	input := string(buf[:size])
	command := input[:1]
	fmt.Printf("Input: [%s]\n", input)

	if command == "q" {
		return Message{Command: buf[0], Slot: 0, Value: ""}, nil
	}

	if len(input) < 4 {
		return Message{}, errors.New("Message is too short")
	}

	if len(input) > 40 {
		return Message{}, errors.New("Message is too long")
	}

	if !SupportedCommands[command] {
		return Message{}, errors.New("command not supported")
	}

	if command == "u" || command == "p" {
		return Message{Command: []byte(command)[0], Slot: 0, Value: input[1:]}, nil
	}

	slot, err := strconv.Atoi(input[1:4])
	if err != nil {
		return Message{}, errors.New("malformed slot")
	}

	var value string
	if command == "w" {
		value = input[4:]
	}

	return Message{Raw: input, Command: []byte(command)[0], Slot: slot, Value: value}, nil
}
