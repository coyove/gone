gone是受[oneindex](https://github.com/donwa/oneindex)启发而开发的，基于golang的OneDrive索引工具。

[Demo](https://one.fib.pw)

1. 使用命令`cp example.conf prod.conf`复制一份新的配置文件
2. 按照oneindex的教程获得ClientID，ClientSecret填入相应字段
2. 准备一个域名example.com并启用HTTPS，将`https://example.com/authcallback`填入RedirURL字段
2. 在Password字段填入一个密码
2. 使用命令`go run *.go -c prod.conf -l :8080`启动gone，反代8080端口至443
2. 打开浏览器访问`https://example.com/?auth=密码`，按照提示授权
2. 完成