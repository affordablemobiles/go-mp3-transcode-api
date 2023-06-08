package main

func assert(i interface{}, err error) interface{} {
	if err != nil {
		panic(err)
	}

	return i
}
