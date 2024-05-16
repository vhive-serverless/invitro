package generator

func StartupLoopConvertMemoryToRuntimeMs(memory int) int {
	// data for AWS from STeLLAR - IISWC'21
	if memory <= 10 {
		return 300
	} else if memory <= 60 {
		return 750
	} else {
		return 1250
	}
}
