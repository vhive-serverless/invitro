package main

func Main(obj map[string]interface{}) map[string]interface{} {
	msg := "You did not tell me who you are."
	name, ok := obj["name"].(string)
	if ok {
		msg = "Hello, " + name + "!"
	}
	result := make(map[string]interface{})
	result["body"] = `<html><body><h3>` + msg + `</h3></body></html>`
	return result
}
