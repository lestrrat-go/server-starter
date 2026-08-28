package statefile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	// These values are defined by Darwin's proc_info API and fcntl.h.
	darwinProcInfoCallPIDInfo   = 2
	darwinProcInfoCallPIDFDInfo = 3
	darwinProcPIDListFDs        = 1
	darwinProcPIDFDVnodeInfo    = 1
	darwinProcFDTypeVnode       = 1
	darwinFWasLocked            = 0x00004000
	darwinProcFDInfoSize        = 8
	darwinVnodeFDInfoSize       = 176
)

type darwinProcFDInfo struct {
	fd     int32
	fdType uint32
}

type darwinProcFileInfo struct {
	openFlags  uint32
	status     uint32
	offset     int64
	fileType   int32
	guardFlags uint32
}

type darwinVnodeStat struct {
	dev           uint32
	mode          uint16
	nlink         uint16
	inode         uint64
	uid           uint32
	gid           uint32
	atime         int64
	atimeNsec     int64
	mtime         int64
	mtimeNsec     int64
	ctime         int64
	ctimeNsec     int64
	birthtime     int64
	birthtimeNsec int64
	size          int64
	blocks        int64
	blksize       int32
	flags         uint32
	gen           uint32
	rdev          uint32
	qspare        [2]int64
}

type darwinVnodeInfo struct {
	stat  darwinVnodeStat
	type_ int32
	pad   int32
	fsid  [2]int32
}

type darwinVnodeFDInfo struct {
	file  darwinProcFileInfo
	vnode darwinVnodeInfo
}

var (
	_ [darwinProcFDInfoSize - int(unsafe.Sizeof(darwinProcFDInfo{}))]struct{}
	_ [int(unsafe.Sizeof(darwinProcFDInfo{})) - darwinProcFDInfoSize]struct{}
	_ [darwinVnodeFDInfoSize - int(unsafe.Sizeof(darwinVnodeFDInfo{}))]struct{}
	_ [int(unsafe.Sizeof(darwinVnodeFDInfo{})) - darwinVnodeFDInfoSize]struct{}
)

func inspectInodeLocks(f *os.File, recordedPID int) (int, bool, error) {
	recordLock := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &recordLock); err != nil {
		return 0, false, err
	}
	if recordLock.Type != syscall.F_UNLCK {
		return 0, true, nil
	}

	err := TryLock(f)
	if err == nil {
		if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
			return 0, false, unlockErr
		}
		return 0, false, nil
	}
	if !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EAGAIN) {
		return 0, false, err
	}

	ownsLock, err := darwinPIDHasLockedVnode(recordedPID, f)
	if err != nil {
		return 0, false, err
	}
	if !ownsLock {
		return 0, false, nil
	}
	return recordedPID, false, nil
}

func darwinPIDHasLockedVnode(pid int, f *os.File) (bool, error) {
	fileInfo, err := f.Stat()
	if err != nil {
		return false, err
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("pid file has unsupported stat data")
	}

	fds, err := darwinPIDFDs(pid)
	if err != nil {
		return false, err
	}
	for _, fd := range fds {
		if fd.fdType != darwinProcFDTypeVnode {
			continue
		}
		var vnode darwinVnodeFDInfo
		used, infoErr := darwinProcInfo(
			darwinProcInfoCallPIDFDInfo,
			pid,
			darwinProcPIDFDVnodeInfo,
			uint64(fd.fd),
			unsafe.Pointer(&vnode),
			int(unsafe.Sizeof(vnode)),
		)
		if errors.Is(infoErr, syscall.EBADF) || errors.Is(infoErr, syscall.ENOENT) {
			continue
		}
		if infoErr != nil {
			return false, infoErr
		}
		if used != int(unsafe.Sizeof(vnode)) {
			continue
		}
		if vnode.vnode.stat.dev != uint32(stat.Dev) || vnode.vnode.stat.inode != stat.Ino {
			continue
		}
		if vnode.file.openFlags&darwinFWasLocked != 0 {
			return true, nil
		}
	}
	return false, nil
}

func darwinPIDFDs(pid int) ([]darwinProcFDInfo, error) {
	size, err := darwinProcInfo(darwinProcInfoCallPIDInfo, pid, darwinProcPIDListFDs, 0, nil, 0)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	entrySize := int(unsafe.Sizeof(darwinProcFDInfo{}))
	fds := make([]darwinProcFDInfo, (size+entrySize-1)/entrySize)
	used, err := darwinProcInfo(
		darwinProcInfoCallPIDInfo,
		pid,
		darwinProcPIDListFDs,
		0,
		unsafe.Pointer(&fds[0]),
		len(fds)*entrySize,
	)
	if err != nil {
		return nil, err
	}
	return fds[:used/entrySize], nil
}

func darwinProcInfo(call, pid, flavor int, arg uint64, buffer unsafe.Pointer, size int) (int, error) {
	result, _, errno := syscall.Syscall6(
		syscall.SYS_PROC_INFO,
		uintptr(call),
		uintptr(pid),
		uintptr(flavor),
		uintptr(arg),
		uintptr(buffer),
		uintptr(size),
	)
	if errno != 0 {
		return 0, errno
	}
	return int(result), nil
}
