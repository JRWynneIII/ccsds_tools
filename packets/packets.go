package packets

import (
	"fmt"
)

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

	MSDUs []MSDU

	Data []byte

	//Custom non-ccsds parameters
	IsCorrupt bool
}

type TransportFileHeader struct {
	Length  uint64
	Counter uint16
}

type TransportFile struct {
	Header TransportFileHeader

	VCDUVersion uint8
	VCID        uint8

	Version             uint8
	Type                bool
	SecondaryHeaderFlag bool
	APID                uint16
	PacketLength        uint16
	Data                []byte
	CRCGood             bool
}

type MSDUHeader struct {
	Version               uint8
	Type                  bool
	SecondaryHeaderFlag   bool //This does not seem to refer to the LRIT secondary headers
	APID                  uint16
	SequenceFlag          uint8
	PacketSequenceCounter uint16
	PacketLength          uint16
}

type MSDU struct {
	Header MSDUHeader
	//CP_PDU Header 6 bytes
	Data []byte

	VCDUVersion uint8
	VCDUSCID    uint8
	VCID        uint8
	VCDUCounter uint32
	VCDUReplay  bool
	CRCGood     bool
}

// Derived from GOESTools
// https://github.com/pietern/goestools/blob/80ece1a7ab8a93fb5dfa50d47387ae7c4a8f2a73/src/assembler/crc.cc
// Copyright (c) 2017, Pieter Noordhuis
func CalcCRCBuffer(data []byte) uint16 {
	crc := uint16(0xFFFF)
	elementIdx := len(data) - 1
	idx := 0

	for elementIdx >= 0 {
		crc = (crc << 8) ^ crcTable[(crc>>8)^uint16(data[idx])]
		idx += 1
		elementIdx -= 1
	}
	return crc
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

func (c *MSDUHeader) IsFillPacket() bool {
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
