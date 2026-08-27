# Use the official Go image as a base
FROM golang:1-alpine

# Set the working directory inside the container
WORKDIR /app

# Copy the Go module files
COPY go.mod ./
# Copy the source code
COPY db-concat.go ./

# Build the db-concat application
# We build it as 'db-concat' (no .exe extension) for Linux environment
RUN go build -o db-concat

# Set the entrypoint to the db-concat executable
ENTRYPOINT ["./db-concat"]

# Instructions for the user on how to use the Docker image:
# To build the Docker image:
# docker build -t db-concat-image .

# To run the db-concat command with a mapped directory and output to stdout:
# docker run --rm -v /path/to/your/project:/data db-concat-image /data/runorder.txt

# To run and output to a file within a mapped directory:
# docker run --rm -v /path/to/your/project:/data db-concat-image --output /data/output.sql /data/runorder.txt

# Replace /path/to/your/project with the actual path on your host machine.
# The /data directory inside the container will contain your project files.