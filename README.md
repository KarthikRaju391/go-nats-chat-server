# go-nats-chat-server
This is a simple chat server developed in go using NATS.

## Starting the Project
1. Start the NATS service on Docker by running the following command:
  `docker run -d -p 4222:4222 -p 8222:8222 -p 6222:6222 --name nats-server -ti nats:latest -js`
2. Run `go run main.go`
3. Open any HTTP client(like Postman) and connect to the following WebSocket URL on two different tabs:
  `ws://localhost:8080/chat/general`
4. Start sending messages with the following payload:
  ```json
    {
      "text": "hello!"
    }
  ```

For a complete tutorial on how this works, check out my [blog](https://medium.com/@karthikraju391/building-a-scalable-chat-server-in-golang-and-nats-9faf68b61d9f).
