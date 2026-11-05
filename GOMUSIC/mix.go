package main

import (
	"fmt"
	"os"
)

func downloadWithRetry(raw string, video bool) (streamSrc, error) {
	var last error
	for i := 0; i < 3; i++ {
		src, err := resolveDirectURL(raw, video)
		if err == nil {
			if src.File != "" && !isHTTP(src.File) {
				st, e := os.Stat(src.File)
				if e != nil || st.Size() < 1024 {
					last = fmt.Errorf("invalid or empty file")
					continue
				}
			}
			return src, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("download failed")
	}
	return streamSrc{}, last
}
