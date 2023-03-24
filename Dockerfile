FROM golang:1.20

WORKDIR /go/src/app
COPY . .

RUN go get google.golang.org/api/drive/v3 \
 && go get golang.org/x/oauth2/google \
 && go get github.com/valyala/fasthttp \
 && go get github.com/qiangxue/fasthttp-routing \
 && go get github.com/ulikunitz/xz \
 && go get github.com/nwaples/rardecode
