package pipeline

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools"
	"github.com/jrwynneiii/goestuner/config"
	"github.com/knadh/koanf/v2"
)

type Pipeline struct {
	Layers              []*ccsds_tools.Layer
	SampleRate          float32
	BufferSize          uint
	ConfigFile          *koanf.Koanf
	NumLayersRegistered int
}

func New(configFile *koanf.koanf) *Pipeline {
	srate := configFile.Float64("radio.sample_rate")
	bufsize := uint(configFile.Int("xrit.chunk_size"))
	return &Pipeline{
		SampleRate: srate,
		BufferSize: bufsize,
		ConfigFile: configFile,
		Layers:     make([]*ccsds_tools.Layer, 6),
	}
}

func (p *Pipeline) Register(id ccsds_tools.LayerType) {
	var layer *ccsds_tools.Layer
	switch id {
	case ccsds_tools.PhysicalLayer:
		input := make(chan []complex64, p.BufferSize)
		output := make(chan byte, p.BufferSize)

		xritConf := config.XRITConf{
			SymbolRate:             configFile.Float64("xrit.symbol_rate"),
			RRCAlpha:               configFile.Float64("xrit.rrc_alpha"),
			RRCTaps:                configFile.Int("xrit.rrc_taps"),
			LowPassTransitionWidth: configFile.Float64("xrit.lowpass_transition_width"),
			PLLAlpha:               float32(configFile.Float64("xrit.pll_alpha")),
			Decimation:             configFile.Int("xrit.decimation_factor"),
			ChunkSize:              uint(configFile.Int("xrit.chunk_size")),
			DoFFT:                  configFile.Bool("xrit.do_fft"),
		}

		agcConf := config.AGCConf{
			Rate:      float32(configFile.Float64("agc.rate")),
			Reference: float32(configFile.Float64("agc.reference")),
			Gain:      float32(configFile.Float64("agc.gain")),
			MaxGain:   float32(configFile.Float64("agc.max_gain")),
		}

		clockConf := config.ClockRecoveryConf{
			Mu:         float32(configFile.Float64("clockrecovery.mu")),
			Alpha:      float32(configFile.Float64("clockrecovery.alpha")),
			OmegaLimit: float32(configFile.Float64("clockrecovery.omega_limit")),
		}

		layer = layers.physical.New(p.SampleRate, p.BufferSize, xritConf, agcConf, clockConf, &input, &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.DataLinkLayer:
		output := make(chan byte, p.BufferSize)

		vitConf := config.ViterbiConf{
			MaxErrors: configFile.Int("viterbi.max_errors"),
		}
		xritConf := config.XRITFrameConf{
			FrameSize:     configFile.Int("xritframe.frame_size"),
			LastFrameSize: configFile.Int("xritframe.last_frame_size"),
		}

		layer = layers.datalink.New(p.BufferSize, vitConf, xritConf, p.Layers[id-1].GetOutput(), &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.TransportLayer:
	case ccsds_tools.SessionLayer:
	case ccsds_tools.PresentationLayer:
	case ccsds_tools.ApplicationLayer:
	default:
		panic(fmt.Errorf("Could not add layer id %d to pipeline", id))
	}
}

func (p *Pipeline) Start() {
	for id, layer := range p.Layers {
		log.Infof("Starting layer: %d", id)
		go layer.Start()
	}
}

func (p *Pipeline) Destroy() {
	for i := p.NumLayersRegistered - 1; i > 0; i-- {
		p.Layers[i].Destroy()
	}
}

func (p *Pipeline) Pause() {
}

func (p *Pipeline) Flush() {
}

func (p *Pipeline) FlushLayer(id ccsds_tools.LayerType) {
}
