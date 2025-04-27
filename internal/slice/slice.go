package slice

func Remove[T any](slice []T, stId int, endId int) []T {
	newSlice := make([]T, len(slice)-endId+stId)

	copy(newSlice, slice[:stId])
	copy(newSlice[stId:], slice[endId:])

	return newSlice
}

func TrimSpaces(line []byte, id int) int {
	for id < len(line) && line[id] == ' ' {
		id++
	}

	return id
}
