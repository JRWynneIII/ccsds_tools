# CCSDS Tools

This library is a work in progress, and intends to provides a simple black-box for processing HRIT/LRIT signals and producing packets. The design directly relates to the CCSDS OSI model, with specific modifications for the GOES mission data format.

## Dependencies

* `libsathelper`: See [here](https://github.com/opensatelliteproject/libsathelper/blob/master/README.md) and [here](https://github.com/JRWynneIII/goestuner?tab=readme-ov-file#installation) for build instructions
* `libcorrect`: See above
* `go`: version 1.18+

## Caveats

Since this library was very quickly thrown together to support multiple similar projects, theres a few unfinished parts and caveats:
* The library supports (and depends on) the `koanf` package to provide runtime configuration data. This approach was chosen to minimize work needed to break this out into a common library
* When calling `GetInput()` or `GetOutput()` on a `Layer`, it returns a generic `any`. You will need to infer the correct type of the channel, since each layer can take or output a different type

### Acknowledgements:

I'd like to thank the [Open Satellite Project](https://github.com/opensatelliteproject) for creating `libsathelper` (Copyright 2016 Lucas Teske), and `SatHelperApp` (Copyright (c) 2018 Open Satellite Project); these two projects were extremely helpful in the development of the demodulator and decoder, and served, not only as a good reference point for development of `ccsds_tools`, but as a wonderful reference for learning various concepts about SDR and xRIT programming. 
