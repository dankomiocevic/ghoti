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

	if command != "r" && command != "w" {
		return nil, errors.New("Command not supported")
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
