gone是受[oneindex](https://github.com/donwa/oneindex)启发而开发的，基于golang的OneDrive索引工具。

[Demo](https://one.fib.pw)

1. 使用命令`cp example.conf prod.conf`复制一份新的配置文件
2. 准备一个域名`example.com`并启用HTTPS，将`https://example.com/authcallback`填入prod.conf里的RedirURL字段
2. 访问`https://apps.dev.microsoft.com/#/appList`进行`添加应用`
2. 将`应用程序ID`填入prod.conf的ClientID字段
2. `生成新密码`，将值填入prod.conf的ClientSecret字段
2. `添加平台`，选择`Web`，将`https://example.com/authcallback`填入`重定向 URL`
2. 保存修改
2. 在prod.conf里的Password字段内填入一个密码，该步骤必须
2. 使用命令`go run *.go -c prod.conf -l :8080`启动gone，反代8080端口`https://example.com`
2. 打开浏览器访问`https://example.com/?auth=密码`，按照提示授权
2. 完成

## 配置文件

配置选项：

1. `Header`: `string`: 指定header.html的路径
2. `Footer`: `string`: 指定footer.html的路径
2. `Ignore`: `string`: 指定哪些文件**不**被显示的文件名正则表达式
2. `Prefetch`: `string`: 指定哪些文件可以被本地缓存的文件名正则表达式
2. `Favicon`: `string`: 指定favicon的路径
2. `DisableReadme`: `bool`: 不渲染readme
2. `CacheSize`: `int`: 目录缓存大小
2. `CacheTTL`: `int`: 目录缓存有效期
2. `PrefetchSize`: `int`: 本地缓存大小，单位为MB