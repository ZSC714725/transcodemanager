// Copyright (c) 2026 Kevin Zang (kevinzang). All rights reserved.
// Use of this source code is governed by the MIT License.
//
// TranscodeManager - FFmpeg 转码任务管理工具
//
// Package process wraps exec.Cmd for controlling an FFmpeg process.

package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Process represents a process
type Process interface {
	Status() Status
	Start() error
	Stop(wait bool) error
	Kill(wait bool) error
	IsRunning() bool
}

// Config for a process
type Config struct {
	Binary         string
	Args           []string
	Reconnect      bool
	ReconnectDelay time.Duration
	StaleTimeout   time.Duration
	Parser         Parser
	OnStart        func()
	OnExit         func()
	OnStateChange  func(from, to string)
	Logger         Logger
}

// Status of a process
type Status struct {
	State    string
	States   States
	Order    string
	Duration time.Duration
	Time     time.Time
	CPU      struct {
		Current float64
		Limit   float64
	}
	Memory struct {
		Current uint64
		Limit   uint64
	}
}

// States cumulative counts
type States struct {
	Finished  uint64
	Starting  uint64
	Running   uint64
	Finishing uint64
	Failed    uint64
	Killed    uint64
}

// Logger interface
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

type stateType string

const (
	stateFinished  stateType = "finished"
	stateStarting  stateType = "starting"
	stateRunning   stateType = "running"
	stateFinishing stateType = "finishing"
	stateFailed    stateType = "failed"
	stateKilled    stateType = "killed"
)

func (s stateType) String() string { return string(s) }

func (s stateType) IsRunning() bool {
	return s == stateStarting || s == stateRunning || s == stateFinishing
}

type process struct {
	binary string
	args   []string
	cmd    *exec.Cmd
	pid    int32
	stdout io.ReadCloser
	lastLine string

	state struct {
		state  stateType
		time   time.Time
		states States
		lock   sync.Mutex
	}
	order struct {
		order string
		lock  sync.Mutex
	}
	parser Parser
	stale struct {
		last    time.Time
		timeout time.Duration
		cancel  context.CancelFunc
		lock    sync.Mutex
	}
	reconn struct {
		enable bool
		delay  time.Duration
		timer  *time.Timer
		lock   sync.Mutex
	}
	killTimer     *time.Timer
	killTimerLock sync.Mutex
	logger        Logger
	limits        Limiter
	callbacks     struct {
		onStart       func()
		onExit        func()
		onStateChange func(from, to string)
		lock          sync.Mutex
	}
}

// New creates a new process
func New(config Config) (Process, error) {
	p := &process{
		binary: config.Binary,
		args:   config.Args,
		parser: config.Parser,
		logger: config.Logger,
		limits: NewSysLimiter(),
	}

	if len(p.binary) == 0 {
		return nil, fmt.Errorf("no valid binary given")
	}

	if p.parser == nil {
		p.parser = &nullParser{}
	}

	if p.logger == nil {
		p.logger = &nopLogger{}
	}

	p.order.order = "stop"
	p.initState(stateFinished)
	p.reconn.enable = config.Reconnect
	p.reconn.delay = config.ReconnectDelay
	p.stale.last = time.Now()
	p.stale.timeout = config.StaleTimeout
	p.callbacks.onStart = config.OnStart
	p.callbacks.onExit = config.OnExit
	p.callbacks.onStateChange = config.OnStateChange

	return p, nil
}

func (p *process) initState(state stateType) {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()
	p.state.state = state
	p.state.time = time.Now()
}

func (p *process) setState(state stateType) error {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()

	prevState := p.state.state
	failed := false

	switch p.state.state {
	case stateFinished:
		if state == stateStarting {
			p.state.state = state
			p.state.states.Starting++
		} else {
			failed = true
		}
	case stateStarting:
		switch state {
		case stateFinishing, stateRunning, stateFailed:
			p.state.state = state
			if state == stateFinishing {
				p.state.states.Finishing++
			} else if state == stateRunning {
				p.state.states.Running++
			} else {
				p.state.states.Failed++
			}
		default:
			failed = true
		}
	case stateRunning:
		switch state {
		case stateFinished, stateFinishing, stateFailed, stateKilled:
			p.state.state = state
			switch state {
			case stateFinished:
				p.state.states.Finished++
			case stateFinishing:
				p.state.states.Finishing++
			case stateFailed:
				p.state.states.Failed++
			case stateKilled:
				p.state.states.Killed++
			}
		default:
			failed = true
		}
	case stateFinishing:
		switch state {
		case stateFinished, stateFailed, stateKilled:
			p.state.state = state
			if state == stateFinished {
				p.state.states.Finished++
			} else if state == stateFailed {
				p.state.states.Failed++
			} else {
				p.state.states.Killed++
			}
		default:
			failed = true
		}
	case stateFailed, stateKilled:
		if state == stateStarting {
			p.state.state = state
			p.state.states.Starting++
		} else {
			failed = true
		}
	default:
		return fmt.Errorf("unhandled state: %s", p.state.state)
	}

	if failed {
		return fmt.Errorf("can't change from %s to %s", p.state.state, state)
	}

	p.state.time = time.Now()
	if p.callbacks.onStateChange != nil {
		go p.callbacks.onStateChange(prevState.String(), p.state.state.String())
	}
	return nil
}

func (p *process) getState() stateType {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()
	return p.state.state
}

func (p *process) isRunning() bool {
	return p.getState().IsRunning()
}

func (p *process) getStateString() string {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()
	return p.state.state.String()
}

func (p *process) Status() Status {
	cpu, memory := p.limits.Current()
	cpuLimit, memoryLimit := p.limits.Limits()

	p.state.lock.Lock()
	stateTime := p.state.time
	stateString := p.state.state.String()
	states := p.state.states
	p.state.lock.Unlock()

	p.order.lock.Lock()
	order := p.order.order
	p.order.lock.Unlock()

	s := Status{
		State:    stateString,
		States:   states,
		Order:    order,
		Duration: time.Since(stateTime),
		Time:     stateTime,
	}
	s.CPU.Current = cpu
	s.CPU.Limit = cpuLimit
	s.Memory.Current = memory
	s.Memory.Limit = memoryLimit
	return s
}

func (p *process) IsRunning() bool {
	return p.isRunning()
}

func (p *process) Start() error {
	p.order.lock.Lock()
	defer p.order.lock.Unlock()

	if p.order.order == "start" {
		return nil
	}
	p.order.order = "start"
	return p.start()
}

func (p *process) start() error {
	if p.isRunning() {
		return nil
	}

	p.unreconnect()
	p.setState(stateStarting)

	var err error
	p.cmd = exec.Command(p.binary, p.args...)
	p.cmd.Env = []string{}

	p.stdout, err = p.cmd.StderrPipe()
	if err != nil {
		p.setState(stateFailed)
		p.parser.Parse(err.Error())
		p.reconnect()
		return err
	}

	if err := p.cmd.Start(); err != nil {
		p.setState(stateFailed)
		p.parser.Parse(err.Error())
		p.reconnect()
		return err
	}

	p.pid = int32(p.cmd.Process.Pid)
	p.limits.Start(int(p.pid))

	p.setState(stateRunning)

	if p.callbacks.onStart != nil {
		go p.callbacks.onStart()
	}

	go p.reader()

	if p.stale.timeout != 0 {
		p.stale.lock.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		p.stale.cancel = cancel
		p.stale.lock.Unlock()
		go p.staler(ctx)
	}

	return nil
}

func (p *process) Stop(wait bool) error {
	p.order.lock.Lock()
	defer p.order.lock.Unlock()

	if p.order.order == "stop" {
		return nil
	}
	p.order.order = "stop"
	return p.stop(wait)
}

func (p *process) Kill(wait bool) error {
	if !p.isRunning() {
		return nil
	}
	p.order.lock.Lock()
	defer p.order.lock.Unlock()
	return p.stop(wait)
}

func (p *process) stop(wait bool) error {
	if !p.isRunning() {
		p.unreconnect()
		return nil
	}
	if p.getState() == stateFinishing {
		return nil
	}

	p.setState(stateFinishing)

	wg := sync.WaitGroup{}
	if wait {
		wg.Add(1)
		p.callbacks.lock.Lock()
		cb := p.callbacks.onExit
		p.callbacks.onExit = func() {
			if cb != nil {
				cb()
			}
			wg.Done()
		}
		p.callbacks.lock.Unlock()
	}

	var err error
	if runtime.GOOS == "windows" {
		err = p.cmd.Process.Kill()
	} else {
		err = p.cmd.Process.Signal(os.Interrupt)
		if err != nil {
			err = p.cmd.Process.Kill()
		} else {
			p.killTimerLock.Lock()
			p.killTimer = time.AfterFunc(5*time.Second, func() {
				p.cmd.Process.Kill()
			})
			p.killTimerLock.Unlock()
		}
	}

	if err == nil && wait {
		wg.Wait()
	}

	if err != nil {
		p.parser.Parse(err.Error())
		p.setState(stateFailed)
	}
	return err
}

func (p *process) reconnect() {
	if !p.reconn.enable {
		return
	}
	p.unreconnect()

	p.reconn.lock.Lock()
	defer p.reconn.lock.Unlock()

	p.reconn.timer = time.AfterFunc(p.reconn.delay, func() {
		p.order.lock.Lock()
		defer p.order.lock.Unlock()
		p.start()
	})
}

func (p *process) unreconnect() {
	p.reconn.lock.Lock()
	defer p.reconn.lock.Unlock()

	if p.reconn.timer != nil {
		p.reconn.timer.Stop()
		p.reconn.timer = nil
	}
}

func (p *process) staler(ctx context.Context) {
	p.stale.lock.Lock()
	p.stale.last = time.Now()
	p.stale.lock.Unlock()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			p.stale.lock.Lock()
			last := p.stale.last
			timeout := p.stale.timeout
			p.stale.lock.Unlock()

			if t.Sub(last).Seconds() > timeout.Seconds() {
				p.stop(false)
				return
			}
		}
	}
}

func (p *process) reader() {
	scanner := bufio.NewScanner(p.stdout)
	scanner.Split(scanLine)

	p.parser.ResetStats()
	p.parser.ResetLog()

	for scanner.Scan() {
		line := scanner.Text()
		p.lastLine = line
		n := p.parser.Parse(line)
		if n != 0 {
			p.stale.lock.Lock()
			p.stale.last = time.Now()
			p.stale.lock.Unlock()
		}
	}

	p.waiter()
}

func (p *process) waiter() {
	if err := p.cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			status := exiterr.Sys().(syscall.WaitStatus)
			if status.Exited() {
				if status.ExitStatus() == 255 {
					p.setState(stateFinished)
				} else {
					p.setState(stateFailed)
				}
			} else {
				p.setState(stateKilled)
			}
		} else {
			p.setState(stateKilled)
		}
	} else {
		p.setState(stateFinished)
	}

	p.limits.Stop()

	p.killTimerLock.Lock()
	if p.killTimer != nil {
		p.killTimer.Stop()
		p.killTimer = nil
	}
	p.killTimerLock.Unlock()

	p.stale.lock.Lock()
	if p.stale.cancel != nil {
		p.stale.cancel()
		p.stale.cancel = nil
	}
	p.stale.lock.Unlock()

	p.parser.ResetStats()

	p.callbacks.lock.Lock()
	if p.callbacks.onExit != nil {
		go p.callbacks.onExit()
	}
	p.callbacks.lock.Unlock()

	p.order.lock.Lock()
	defer p.order.lock.Unlock()

	if p.order.order == "start" {
		p.reconnect()
	}
}

func scanLine(data []byte, atEOF bool) (advance int, token []byte, err error) {
	start := 0
	for start < len(data) {
		r, w := utf8.DecodeRune(data[start:])
		if r != '\n' && r != '\r' {
			break
		}
		start += w
	}

	for i := start; i < len(data); {
		r, w := utf8.DecodeRune(data[i:])
		if r == '\n' || r == '\r' {
			return i + w, data[start:i], nil
		}
		i += w
	}

	if atEOF && len(data) > start {
		return len(data), data[start:], nil
	}
	return start, nil, nil
}

type nullParser struct{}

func (p *nullParser) Parse(line string) uint64       { return 1 }
func (p *nullParser) ResetStats()                    {}
func (p *nullParser) ResetLog()                     {}
func (p *nullParser) Log() []Line                   { return nil }

type nopLogger struct{}

func (l *nopLogger) Info(format string, args ...interface{})  {}
func (l *nopLogger) Error(format string, args ...interface{}) {}
func (l *nopLogger) Debug(format string, args ...interface{}) {}
