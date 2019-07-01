# DO NOT REMOVE! The following line informs gocdk of the Docker image's name.
# gocdk-image: {{.ProjectName}}

# Step 1: Build Go binary.
FROM golang:1.12 as build
ENV GO111MODULE on
ENV GOPROXY https://proxy.golang.org/
# Warm up the module cache.
# Only copy in go.mod and go.sum to increase Docker cache hit rate.
COPY go.mod go.sum /warmup/
RUN cd /warmup && go mod download
# Now build the whole tree.
COPY . /src
WORKDIR /src
RUN go build -o {{.ProjectName}}

# Step 2: Create image with built Go binary.
FROM gcr.io/distroless/base
COPY --from=build /src/{{.ProjectName}} /{{.ProjectName}}
ENV PORT 8080
EXPOSE 8080
ENTRYPOINT ["/{{.ProjectName}}"]
