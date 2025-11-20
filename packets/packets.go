package packets

import (
	"fmt"
)

type LRITFile struct {
	Header LRITHeader
	Data   []LRITBlock
}

// size 8192 bytes
// Constructed from multiple VCDU's
type LRITBlock struct {
	// 8190 bytes
	Data []byte
	//2 bytes
	CRC uint16
}

type LRITHeader struct {
	FileCounter uint16
	Length      uint64
}

// Size = 892 bytes
type VCDU struct {
	// VCDU Header 6 bytes
	VCDUVersion uint8
	VCDUSCID    uint8
	VCID        uint8
	VCDUCounter uint32
	VCDUReplay  bool

	// M_PDU Header 2 bytes
	FirstHeaderOffset uint16

	MSDUs []CPPDU

	Data []byte

	//Custom non-ccsds parameters
	IsCorrupt bool
}

type TPPDUHeader struct {
	Length  uint64
	Counter uint16
}

type TPPDU struct {
	Header      TPPDUHeader
	VCDUVersion uint8
	VCDUSCID    uint8
	VCID        uint8
	VCDUCounter uint32
	VCDUReplay  bool

	Version               uint8
	Type                  bool
	SecondaryHeaderFlag   bool
	APID                  uint16
	SequenceFlag          uint8
	PacketSequenceCounter uint16
	PacketLength          uint16
	Data                  []byte
	WantCRC               uint16
	HaveCRC               uint16
}

type CPPDUHeader struct {
	Version               uint8
	Type                  bool
	SecondaryHeaderFlag   bool
	APID                  uint16
	SequenceFlag          uint8
	PacketSequenceCounter uint16
	PacketLength          uint16
}

type CPPDU struct {
	Header CPPDUHeader
	//CP_PDU Header 6 bytes
	Data    []byte
	WantCRC uint16
	HaveCRC uint16

	VCDUVersion uint8
	VCDUSCID    uint8
	VCID        uint8
	VCDUCounter uint32
	VCDUReplay  bool
}

// Derived from GOESTools' diffWithWrap() utility
// https://github.com/pietern/goestools/blob/80ece1a7ab8a93fb5dfa50d47387ae7c4a8f2a73/src/assembler/crc.cc
// Copyright (c) 2017, Pieter Noordhuis
func (t *TPPDU) CalcCRC() {
	crc := uint16(0xFFFF)
	elementIdx := len(t.Data) - 1
	idx := 0

	for elementIdx >= 0 {
		crc = (crc << 8) ^ crcTable[(crc>>8)^uint16(t.Data[idx])]
		idx += 1
		elementIdx -= 1
	}
	t.WantCRC = crc
}

// Based upon GOESTools' diffWithWrap() utility
// https://github.com/pietern/goestools/blob/80ece1a7ab8a93fb5dfa50d47387ae7c4a8f2a73/src/goesemwin/qbt.cc#L52
// Copyright (c) 2017, Pieter Noordhuis
func CounterDiff(div, a, b uint32) uint {
	ret := uint(0)
	if a < div && b < div {
		if a <= b {
			ret = uint(b - a)
		} else {
			ret = uint(div - a + b)
		}
	}

	return ret
}

func FrameIsValid(data []byte) bool {
	if len(data) != 892 {
		return false
	}

	return true
}

func (v *VCDU) FHPIsValid() error {
	if v.FirstHeaderOffset > uint16(len(v.Data)-1) {
		v.IsCorrupt = true
		return fmt.Errorf("Invalid FHP found in VCDU (VCID: %d, Counter: %d)", v.VCID, v.VCDUCounter)
	}
	return nil
}

func (v *VCDU) ContainsMSDUHeader() bool {
	if v.FirstHeaderOffset == 2047 {
		return false
	}
	return true
}

func (c *CPPDUHeader) IsFillPacket() bool {
	if c.APID == 2047 {
		return true
	}
	return false
}

func (v *VCDU) IsFillPacket() bool {
	if v.VCID == 63 {
		return true
	}
	return false
}

func (v *VCDU) String() string {
	return fmt.Sprintf("%##v", *v)
}
