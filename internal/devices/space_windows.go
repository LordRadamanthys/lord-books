package devices

import "golang.org/x/sys/windows"

func diskSpace(path string) (free, total uint64) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	if windows.GetDiskFreeSpaceEx(p, &free, &total, nil) != nil {
		return 0, 0
	}
	return free, total
}
