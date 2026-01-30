package pkg

import (
	"fmt"
	"time"

	"github.com/vbauerster/mpb/v8/decor"
)

func Start() int64 {
	return time.Now().UnixMilli()
}

func Ttt(totalData uint64, startTime int64) {
	now := time.Now().UnixMilli()

	diff := float64(now-startTime) / 1000.0

	totalDataMb := float64(totalData) / 1048576.0

	fmt.Printf("\nStats:\n")
	fmt.Printf("Time Taken: %.2f seconds\n", diff)
	fmt.Printf("Total Amount Transfered: % .2f \n", decor.SizeB1024(totalData))
	fmt.Printf("Average Speed: %.2f MiB/s\n", totalDataMb/diff)
}
