package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"

	"github.com/dchest/spipe"
)

var (
	remote  = flag.String("remote", "", "remote address to dial")
	local   = flag.Int("local", 5000, "local port to listen to")
	tcs     = flag.String("tcs", "localhost:30541", "address of the tcs server")
	keyFile = flag.String("key", "", "key file to use a secure connection")
	dumpOut = flag.String("dump-out", "", "file to dump out-bound traffic to")
	dumpIn  = flag.String("dump-in", "", "file to dump out-bound traffic to")
)

var (
	header []byte
	key    []byte
	secure bool
)

func main() {
	flag.Parse()

	if len(*remote) == 0 || len(*tcs) == 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	var err error
	if len(*keyFile) != 0 {
		key, err = ioutil.ReadFile(*keyFile)
		if err != nil {
			log.Fatal(err)
		}
		secure = true
	}

	header = make([]byte, 4+len(*remote))
	binary.LittleEndian.PutUint32(header, uint32(len(*remote)))
	copy(header[4:], *remote)

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", *local))
	if err != nil {
		log.Fatalf("failed to listen to port %d: %v", *local, err)
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}
		go forward(conn)
	}
}
func forward(local io.ReadWriteCloser) {
	defer local.Close()

	var err error
	var remote io.ReadWriteCloser

	if secure {
		remote, err = spipe.Dial(key, "tcp", *tcs)
	} else {
		remote, err = net.Dial("tcp", *tcs)
	}
	if err != nil {
		log.Printf("failed to dial tcs %q: %v", *tcs, err)
		return
	}
	defer remote.Close()

	_, err = remote.Write(header)
	if err != nil {
		log.Printf("failed to write header: %v", err)
		return
	}

	var dumpOutWriter io.WriteCloser
	if len(*dumpOut) != 0 {
		dumpOutWriter, err = os.OpenFile(*dumpOut, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
		if err != nil {
			log.Printf("failed to open dump out file %q: %v", *dumpOut, err)
			return
		}
		defer dumpOutWriter.Close()
	}

	var dumpInWriter io.WriteCloser
	if len(*dumpIn) != 0 {
		dumpInWriter, err = os.OpenFile(*dumpIn, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0777)
		if err != nil {
			log.Printf("failed to open dump in file %q: %v", *dumpIn, err)
			return
		}
		defer dumpInWriter.Close()
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		if dumpInWriter != nil {
			io.Copy(local, io.TeeReader(remote, dumpInWriter))
		} else {
			io.Copy(local, remote)
		}
		wg.Done()
		local.Close()
	}()

	go func() {
		if dumpOutWriter != nil {
			io.Copy(remote, io.TeeReader(local, dumpOutWriter))
		} else {
			io.Copy(remote, local)
		}
		wg.Done()
		remote.Close()
	}()
	wg.Wait()
}
