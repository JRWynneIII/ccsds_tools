package packets

import (
	"encoding/binary"
	"fmt"
)

type LRITFile struct {
	Data []LRITBlock
}

// size 8192 bytes
// Constructed from multiple VCDU's
type LRITBlock struct {
	// 8190 bytes
	Data []byte
	//2 bytes
	CRC uint16
}

// Size = 892 bytes
type VCDU struct {
	Header   VCDUPrimaryHeader // 6 bytes
	DataZone M_PDU             // 886 bytes
}

// Size = 886 bytes
type M_PDU struct {
	Header M_PDUHeader // 2 bytes
	Data   []byte      // 884 bytes
}

// Size = 2 bytes
type M_PDUHeader struct {
	FirstHeaderPointer int // If == 2047, then all data in this packet belongs to previous block
}

type CP_PDU struct {
	// Size of following is 6 bytes
	Version               int
	Type                  bool
	SecondaryHeaderFlag   bool
	APID                  int
	SequenceFlag          int
	PacketSequencyCounter int
	PacketLength          int
	// Variable! 1 - 8192 bytes!!!
	// UserData can span multiple M_PDU blocks!!! This is WEIRD
	UserData []byte
}

// Size = 6 bytes
type VCDUPrimaryHeader struct {
	Version int
	SCID    int
	VCID    int
	Counter int
	Replay  bool
}

func (v *VCDU) String() string {
	return fmt.Sprintf("%##v", *v)
}

func ParseFrame(data []byte) *VCDU {
	header := data[:6]
	datazone := data[6:]
	v := VCDU{
		Header:   CreateVCDUPrimaryHeader(header),
		DataZone: CreateM_PDU(datazone),
	}
	return &v
}

func CreateVCDUPrimaryHeader(data []byte) VCDUPrimaryHeader {
	//data length = 6 bytes
	version := (data[0] & 0b11000000) >> 6
	id := binary.BigEndian.Uint16(data[:2]) & 0b0011111111111111
	scid := (id & 0b001111111100000000) >> 8
	vcid := id & 0b000000000011111111
	counter := binary.BigEndian.Uint16(data[2:5])
	replay := ((data[5] & 0b10000000) >> 7) > 0

	return VCDUPrimaryHeader{
		Version: int(version),
		SCID:    int(scid),
		VCID:    int(vcid),
		Counter: int(counter),
		Replay:  replay,
	}
}

func CreateM_PDUHeader(data []byte) M_PDUHeader {
	//data length = 2 bytes
	return M_PDUHeader{
		FirstHeaderPointer: int(binary.BigEndian.Uint16(data) & 0b0000011111111111),
	}
}

func CreateM_PDU(data []byte) M_PDU {
	//data length = 886 bytes
	header := data[:2]
	packetzone := data[2:]
	return M_PDU{
		Header: CreateM_PDUHeader(header),
		Data:   packetzone,
	}
}
