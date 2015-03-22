package ghttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrAlreadyClosed        = errors.New("Listener already closed")
	errRestartListener      = errors.New("No listener for restart")
	errListenerCloseTimeout = errors.New("Listener close timeout")
	gs                      = GraceServer{ListenerCloseTimeout: 60 * time.Second}
)

const (
	envRestartKey       = "_GRACE_RESTART"
	envRestartKeyPrefix = envRestartKey + "="
	errClosed           = "use of closed network connection"
)

type conn struct {
	net.Conn
	wg   *sync.WaitGroup
	once sync.Once
}

func (c *conn) Close() error {
	defer c.once.Do(c.wg.Done)
	return c.Conn.Close()
}

// GracableListener requires the file descriptor of listener could be got by File() function.
// When service restarts, the listener will be passed to child process by file descriptor.
// So only TCPListener or UNIXListener is supported.
type GracableListener interface {
	net.Listener                   // Inherit original TCP/UNIX listener interface
	File() (f *os.File, err error) // Get file descriptor
	SetDeadline(t time.Time) error // Needed by close listener
}

// InheritListener inherits listener from old processor.
// File descriptor number of listener is 3 for only stdin, stdout, stderr and listener
// are opened when restart.
func InheritListener() (l GracableListener, err error) {
	isRestart := os.Getenv(envRestartKey)
	if isRestart == "1" {
		f := os.NewFile(uintptr(3), "listener")
		tmp, err := net.FileListener(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		return tmp.(GracableListener), nil
	} else {
		return nil, errRestartListener
	}
}

func GetListener(addr string) (l GracableListener, err error) {
	l, err = InheritListener()
	if err != nil {
		tcpAddr, e := net.ResolveTCPAddr("tcp", addr)
		if e != nil {
			return nil, e
		}
		l, e = net.ListenTCP("tcp", tcpAddr)
		if e != nil {
			return nil, e
		}
		err = nil
	}
	return
}

func SetListenerCloseTimeout(seconds int64) {
	gs.ListenerCloseTimeout = time.Duration(seconds) * time.Second
}

func ListenAndServe(addr string, handler http.Handler) (err error) {
	return gs.ListenAndServe(addr, handler)
}

func Serve(l GracableListener, handler http.Handler) (err error) {
	return gs.Serve(l, handler)
}

type gListener struct {
	GracableListener
	closed      bool
	closedMutex sync.RWMutex
	wg          sync.WaitGroup
}

func newGListener(l GracableListener) (gl *gListener) {
	gl = new(gListener)
	gl.GracableListener = l
	return
}

func (l *gListener) Close() error {
	l.closedMutex.Lock()
	l.closed = true
	l.closedMutex.Unlock()

	var err error
	if os.Getppid() == 1 {
		err = l.GracableListener.SetDeadline(time.Now())
	} else {
		err = l.GracableListener.Close()
	}
	l.wg.Wait()
	return err
}

func (l *gListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.wg.Add(1)
	defer func() {
		if c == nil {
			l.wg.Done()
		}
	}()

	l.closedMutex.RLock()
	if l.closed {
		l.closedMutex.RUnlock()
		return nil, ErrAlreadyClosed
	}
	l.closedMutex.RUnlock()

	c, err := l.GracableListener.Accept()
	if err != nil {
		if strings.HasSuffix(err.Error(), errClosed) {
			return nil, ErrAlreadyClosed
		}

		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			l.closedMutex.RLock()
			if l.closed {
				l.closedMutex.RUnlock()
				return nil, ErrAlreadyClosed
			}
			l.closedMutex.RUnlock()
		}
		return nil, err
	}
	return &conn{Conn: c, wg: &l.wg}, nil
}

type GraceServer struct {
	ListenerCloseTimeout time.Duration
	l                    *gListener
	srv                  *http.Server
}

func (gs *GraceServer) SetReadTimeout(d time.Duration) {
	gs.srv.ReadTimeout = d
}

func (gs *GraceServer) SetWriteTimeout(d time.Duration) {
	gs.srv.WriteTimeout = d
}

func (gs *GraceServer) SetMaxHeaderBytes(n int) {
	gs.srv.MaxHeaderBytes = n
}

func (gs *GraceServer) Serve(l GracableListener, handler http.Handler) (err error) {
	gs.l = newGListener(l)
	gs.srv = &http.Server{Handler: handler}

	waitServeErr := make(chan error, 1)
	go func() {
		waitServeErr <- gs.srv.Serve(gs.l) // close listener 1
		close(waitServeErr)
	}()

	isRestart := os.Getenv(envRestartKey)
	if isRestart == "1" {
		gs.CloseParentServer()
	}

	waitSignalErr := make(chan error, 1)
	go func() {
		waitSignalErr <- gs.WaitSignal()
		close(waitSignalErr)
	}()

	select {
	case err = <-waitServeErr:
		if err == ErrAlreadyClosed {
			err = nil
		}
		return
	case err = <-waitSignalErr:
		return
	}

}

func (gs *GraceServer) ListenAndServe(addr string, handler http.Handler) (err error) {
	l, err := GetListener(addr)
	if err != nil {
		return
	}
	return gs.Serve(l, handler)
}

// CloseParentService Send TERM signal to old service processor.
func (gs *GraceServer) CloseParentServer() error {
	parentPID := os.Getppid()
	if parentPID == 1 {
		return nil
	}
	return syscall.Kill(parentPID, syscall.SIGQUIT)
}

func SrvCtrlhandler(w http.ResponseWriter, req *http.Request) {
	var err error
	err = req.ParseForm()
	if err != nil {
		fmt.Fprint(w, err.Error())
		return
	}

	action := req.Form.Get("action")
	switch action {
	case "restart":
		err = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	case "stop":
		err = syscall.Kill(os.Getpid(), syscall.SIGQUIT)
	}
	if err != nil {
		fmt.Fprint(w, err.Error())
	} else {
		fmt.Fprint(w, action+" success")
	}
}

func (gs *GraceServer) closeListener() (err error) {
	gs.srv.SetKeepAlivesEnabled(false)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = gs.l.Close() // close listener 2
		wg.Done()
	}()

	if gs.ListenerCloseTimeout == 0 {
		// wait forever... T_T
		wg.Wait()
	} else {
		// wait in background to allow for implementing a timeout
		done := make(chan struct{})
		go func() {
			defer close(done)
			wg.Wait()
		}()

		// wait for graceful termination or timeout
		select {
		case <-done:
			// fmt.Println("wg.Wait done")
		case <-time.After(gs.ListenerCloseTimeout):
			return errListenerCloseTimeout
		}
	}

	return
}

func (gs *GraceServer) restart() (err error) {
	if gs.l == nil {
		return errRestartListener
	}

	// Extract the file descriptor from the listener.
	f, err := gs.l.GracableListener.File()
	if err != nil {
		return err
	}
	defer f.Close()                  // Close listener file descriptor when old processor exit
	syscall.CloseOnExec(int(f.Fd())) // Make sure file descriptor for listener in new process is closed

	// Use the original binary location. This works with symlinks such that if
	// the file it points to has been changed we will use the updated symlink.
	argv0, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}

	// In order to keep the working directory the same as when we started.
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	var env []string
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, envRestartKeyPrefix) {
			env = append(env, v)
		}
	}
	env = append(env, fmt.Sprintf("%s%d", envRestartKeyPrefix, 1))

	allFiles := append([]*os.File{os.Stdin, os.Stdout, os.Stderr}, f)
	_, err = os.StartProcess(argv0, os.Args, &os.ProcAttr{
		Dir:   wd,
		Env:   env,
		Files: allFiles,
	})
	return err
}

// WaitSignal waits for signals to gracefully terminate or restart the process.
// When code runs as a daemon process, it is often hoped running without any stop.
// So some works should be done on the flying.
// Some signals suggestion usage are listed herein.
func (gs *GraceServer) WaitSignal() error {
	ch := make(chan os.Signal, 6)
	signal.Notify(ch,
		syscall.SIGTERM, // TERM : Exit immediatly
		syscall.SIGINT,  // INT  : Exit immediatly
		syscall.SIGQUIT, // QUIT : Exit gracefully
		syscall.SIGHUP,  // HUP  : Gracefully reload configure and restart
	// USR1 : Reopen log file
	// USR2 : Update gracefully
	// SIGWINCH : Exit worker process gracefully
	// SIGSTOP, SIGKILL : Need not captured at anytime, process will quit immediatly
	)

	for {
		sig := <-ch
		switch sig {
		case syscall.SIGTERM:
			fallthrough
		case syscall.SIGINT:
			signal.Stop(ch)
			return nil
		case syscall.SIGQUIT:
			signal.Stop(ch)
			return gs.closeListener()
		case syscall.SIGHUP:
			// we only return here if there's an error, otherwise the new process
			// will send us a TERM when it's ready to trigger the actual shutdown.
			if err := gs.restart(); err != nil {
				return err
			}
		}
	}
}
