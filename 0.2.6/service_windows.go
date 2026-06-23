//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"golang.org/x/sys/windows/svc"
)

func runAgent(cfg agentConfig, logger *log.Logger) error {
	interactive, err := svc.IsAnInteractiveSession()
	if err != nil {
		return err
	}
	if interactive {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return runAgentLoop(ctx, cfg, logger)
	}
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName()
	}
	return svc.Run(serviceName, &windowsAgentService{cfg: cfg, logger: logger})
}

type windowsAgentService struct {
	cfg    agentConfig
	logger *log.Logger
}

func (s *windowsAgentService) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runAgentLoop(ctx, s.cfg, s.logger)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				changes <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				err := <-errCh
				if err != nil {
					s.logger.Printf("service stop failed: %v", err)
					return false, 1
				}
				return false, 0
			default:
			}
		case err := <-errCh:
			if err != nil {
				s.logger.Printf("service runtime failed: %v", err)
				return false, 1
			}
			return false, 0
		}
	}
}
