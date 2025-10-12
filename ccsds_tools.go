package ccsds_tools

type LayerType int

// IDs for our layer types
const (
	PhysicalLayer LayerType = iota
	DataLinkLayer
	TransportLayer
	SessionLayer
	PresentationLayer
	ApplicationLayer
)

type Layer[T any] interface {
	GetInput() *chan T
	GetOutput() *chan T
	Start()
	Destroy()
}
