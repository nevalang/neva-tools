package main

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) Initialize(glspCtx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	if params.RootPath != nil {
		s.workspacePath = *params.RootPath
	} else if params.RootURI != nil {
		path, err := uriToPath(string(*params.RootURI))
		if err == nil {
			s.workspacePath = path
		}
	}

	capabilities := s.handler.CreateServerCapabilities()
	if capabilities.CodeLensProvider != nil {
		capabilities.CodeLensProvider.ResolveProvider = ptr(true)
	}
	// Populate the legend so clients can map token type indices from responses.
	if opts, ok := capabilities.SemanticTokensProvider.(*protocol.SemanticTokensOptions); ok {
		opts.Legend = semanticTokensLegend()
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    s.name,
			Version: &s.version,
		},
	}, nil
}

func (s *Server) Initialized(glspCtx *glsp.Context, params *protocol.InitializedParams) error {
	return s.indexAndNotifyProblems(glspCtx.Notify)
}

func (s *Server) Shutdown(context *glsp.Context) error {
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (s Server) Exit(_ *glsp.Context) error {
	return nil
}

func (s Server) SetTrace(_ *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}
