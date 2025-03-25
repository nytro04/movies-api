FROM golang:1.22-alpine

# Set the Current Working Directory inside the container
WORKDIR /usr/src/app

# copy source code
COPY . .

# download and install dependencies
RUN go mod download && go mod verify && go mod tidy && go mod vendor

# build the go app
RUN GOOS=linux GOARCH=amd64/linux go build -o api ./cmd/api

# Expose port 4000 to the outside world
EXPOSE 8888

# Command to run the executable
# CMD [./cmd/api]