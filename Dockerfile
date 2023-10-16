# Use the official Golang image as a base image
FROM golang:1.21

# Set the working directory in the container
WORKDIR /app

# Copy the Go code to the container's working directory
COPY . .

# Install any dependencies (assuming you're using go modules)
RUN go mod download

# Compile the Go code
RUN go build -o node-brainer

# Specify the command to run on container start
ENTRYPOINT ["./node-brainer"]
