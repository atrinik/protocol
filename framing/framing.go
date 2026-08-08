// Copyright 2026 The Atrinik Project
// SPDX-License-Identifier: MIT

// Package framing implements Game Protocol 1's canonical unsigned-LEB128
// stream frames. It does not decode or authorize Protobuf messages.
package framing

import (
	"errors"
	"io"
)

const (
	MaximumGameplayFrame = uint32(1 << 20)
	MaximumResourceFrame = uint32(4 << 20)
)

var (
	ErrEmptyFrame       = errors.New("game protocol frame is empty")
	ErrFrameTooLarge    = errors.New("game protocol frame exceeds the role limit")
	ErrLengthOverflow   = errors.New("game protocol frame length overflows uint32")
	ErrNonCanonicalSize = errors.New("game protocol frame length is not canonical")
)

// Read reads one complete frame. It validates the length before allocating and
// never returns a partial payload.
func Read(reader io.Reader, limit uint32) ([]byte, error) {
	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, ErrEmptyFrame
	}
	if length > limit {
		return nil, ErrFrameTooLarge
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// Write writes one complete canonical frame.
func Write(writer io.Writer, payload []byte, limit uint32) error {
	if len(payload) == 0 {
		return ErrEmptyFrame
	}
	if uint64(len(payload)) > uint64(limit) {
		return ErrFrameTooLarge
	}

	var prefix [5]byte
	count := putLength(prefix[:], uint32(len(payload)))
	if err := writeAll(writer, prefix[:count]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func writeAll(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := writer.Write(value)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(value) {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func readLength(reader io.Reader) (uint32, error) {
	var value uint32
	var single [1]byte
	for index := uint32(0); index < 5; index++ {
		if _, err := io.ReadFull(reader, single[:]); err != nil {
			return 0, err
		}
		current := single[0]
		if index == 4 && current > 0x0f {
			return 0, ErrLengthOverflow
		}
		value |= uint32(current&0x7f) << (7 * index)
		if current&0x80 == 0 {
			if index > 0 && current == 0 {
				return 0, ErrNonCanonicalSize
			}
			return value, nil
		}
	}
	return 0, ErrLengthOverflow
}

func putLength(destination []byte, value uint32) int {
	index := 0
	for value >= 0x80 {
		destination[index] = byte(value) | 0x80
		value >>= 7
		index++
	}
	destination[index] = byte(value)
	return index + 1
}
