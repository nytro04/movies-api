FROM golang:1.22-alpine

# Set the Current Working Directory inside the container
WORKDIR /app

# copy source code
COPY . .

# download and install dependencies
RUN go mod download && go mod verify && go mod tidy && go mod vendor

# build the go app
RUN GOOS=linux GOARCH=amd64 go build -o api ./cmd/api

FROM scratch
WORKDIR /app

# Expose port 4000 to the outside world
EXPOSE 4000


# Command to run the executable
CMD ["./cmd/api"]