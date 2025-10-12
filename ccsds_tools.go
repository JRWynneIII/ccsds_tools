package ccsds_tools

type LayerType int

// IDs for our layer types
const (
	PhysicalLayer LayerType = itoa
	DataLinkLayer
	TransportLayer
	SessionLayer
	PresentationLayer
	ApplicationLayer
)

type Layer interface {
	GetInput() *chan any
	GetOutput() *chan any
	Start()
	Destroy()
}
