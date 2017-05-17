/* Example Usage

aw := newAsyncWriter(device, dst, )
go aw.doCopy()

Loop:
for {
        select {
                case <- aw.C:
                        fmt.Printf("transfered %v / %v bytes (%.2f%%)\n",
                                aw.BytesCompleted(),
                                aw.TotalSize,
                                100*aw.Progress())
                case <- aw.Done:
                        break Loop
        }
}
if err := aw.Err(); err != nil {
        log.Fatal(err)
}
*/
package adb

import (
	"io"
	"sync"
	"time"
)

type asyncWriter struct {
	Done           chan bool
	DoneCopy       chan bool // for debug
	C              chan bool
	err            error
	dst            io.WriteCloser
	dstPath        string
	TotalSize      int64
	dev            *Device
	bytesCompleted int64
	copyErrC       chan error
	wg             sync.WaitGroup
}

func newAsyncWriter(dev *Device, dst io.WriteCloser, dstPath string, totalSize int64) *asyncWriter {
	return &asyncWriter{
		Done:      make(chan bool),
		DoneCopy:  make(chan bool, 1),
		C:         make(chan bool),
		dst:       dst,
		dstPath:   dstPath,
		dev:       dev,
		TotalSize: totalSize,
		copyErrC:  make(chan error, 1),
	}
}

func (a *asyncWriter) BytesCompleted() int64 {
	return a.bytesCompleted
}

func (a *asyncWriter) Progress() float64 {
	if (a.TotalSize) == 0 {
		return 0.0
	}
	return float64(a.bytesCompleted) / float64(a.TotalSize)
}

func (a *asyncWriter) Err() error {
	return a.err
}

func (a *asyncWriter) Cancel() error {
	return a.dst.Close()
}

func (a *asyncWriter) Wait() {
	<-a.Done
}

func (a *asyncWriter) doCopy(reader io.Reader) {
	a.wg.Add(1)
	defer a.wg.Done()

	go a.darinProgress()
	written, err := io.Copy(a.dst, reader)
	if err != nil {
		a.err = err
		a.copyErrC <- err
	}
	a.TotalSize = written
	defer a.dst.Close()
	a.DoneCopy <- true
}

func (a *asyncWriter) darinProgress() {
	t := time.NewTicker(time.Millisecond * 500)
	defer func() {
		t.Stop()
		a.wg.Wait()
		a.Done <- true
	}()
	var lastSize int32
	for {
		select {
		case <-t.C:
			finfo, err := a.dev.Stat(a.dstPath)
			if err != nil && !HasErrCode(err, FileNoExistError) {
				a.err = err
				return
			}
			if finfo == nil {
				continue
			}
			if lastSize != finfo.Size {
				lastSize = finfo.Size
				select {
				case a.C <- true:
				default:
				}
			}
			a.bytesCompleted = int64(finfo.Size)
			if a.TotalSize != 0 && a.bytesCompleted >= a.TotalSize {
				return
			}
		case <-a.copyErrC:
			return
		}
	}
}
