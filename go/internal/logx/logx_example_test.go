package logx

import "fmt"

func ExampleParseLine() {
	entry, err := ParseLine(1, "2026-08-18T10:00:00Z INFO api-gateway 15 GET /users/42 -> 200")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v", entry)
	// Output: logx.Entry{Time:time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC), Level:1, Service:"api-gateway", Latency:15000000, Message:"GET /users/42 -> 200"}
}
