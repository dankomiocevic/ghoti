package server

import (
	"errors"
	"fmt"
	"log/slog"
)

const (
	// SlotDigits is the amount of digits used to represent a slot number.
	SlotDigits = 3
	// TotalSlots is the amount of slots available in the server.
	TotalSlots = 1000

	minMessageSize = 1 + SlotDigits
	maxMessageSize = 40
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
	"s": true,
	"d": true,
}

// parseSlot converts the three digit slot number into an integer.
// Only ASCII digits are accepted, so signs, spaces and any other
// representation accepted by strconv.Atoi are rejected here.
func parseSlot(digits string) (int, error) {
	if len(digits) != SlotDigits {
		return 0, errors.New("malformed slot")
	}

	slot := 0
	for i := 0; i < len(digits); i++ {
		digit := digits[i]
		if digit < '0' || digit > '9' {
			return 0, errors.New("malformed slot")
		}
		slot = slot*10 + int(digit-'0')
	}

	if slot < 0 || slot >= TotalSlots {
		return 0, errors.New("slot out of range")
	}
	return slot, nil
}

// credentialCommands are the commands that carry a credential as their
// value. Their argument must never reach the logs.
var credentialCommands = map[byte]bool{
	'u': true,
	'p': true,
}

// redactCredentials returns a representation of the message that is safe to
// log. Commands carrying a credential keep the command byte and the length of
// the value, so a malformed message can still be diagnosed, but the value
// itself is replaced. Every other command is returned untouched.
func redactCredentials(command byte, input string) string {
	if !credentialCommands[command] {
		return input
	}
	return fmt.Sprintf("%c<redacted:%d bytes>", command, len(input)-1)
}

func ParseMessage(size int, buf []byte) (Message, error) {
	if size < 0 || size > len(buf) {
		return Message{}, errors.New("invalid message size")
	}

	input := string(buf[:size])
	if len(input) < 1 {
		return Message{}, errors.New("Message is empty")
	}

	command := input[0]
	slog.Debug("Message received", slog.String("input", redactCredentials(command, input)))

	if command == 'q' {
		if len(input) > 1 {
			return Message{}, errors.New("quit command does not take any argument")
		}
		return Message{Command: command, Slot: 0, Value: ""}, nil
	}

	if len(input) < minMessageSize {
		return Message{}, errors.New("Message is too short")
	}

	if len(input) > maxMessageSize {
		return Message{}, errors.New("Message is too long")
	}

	if !SupportedCommands[string(command)] {
		return Message{}, errors.New("command not supported")
	}

	if command == 'u' || command == 'p' {
		return Message{Command: command, Slot: 0, Value: input[1:]}, nil
	}

	// Every remaining command carries a slot number. Only the write command
	// takes a value after it, the others must not have any trailing bytes.
	if command != 'w' && len(input) != minMessageSize {
		return Message{}, errors.New("unexpected data after the slot number")
	}

	slot, err := parseSlot(input[1:minMessageSize])
	if err != nil {
		return Message{}, err
	}

	var value string
	if command == 'w' {
		value = input[minMessageSize:]
	}

	return Message{Raw: input, Command: command, Slot: slot, Value: value}, nil
}
