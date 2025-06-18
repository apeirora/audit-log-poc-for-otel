# What has been observed so far

## when the collector can't be reached

io.opentelemetry.exporter.sender.okhttp.internal.RetryInterceptor.intercept(RetryInterceptor.java:96) ~[opentelemetry-exporter-sender-okhttp-1.49.0.jar:1.49.0]

```text
java.io.InterruptedIOException: timeout
	at okhttp3.internal.connection.RealCall.timeoutExit(RealCall.kt:398) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.connection.RealCall.callDone(RealCall.kt:360) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.connection.RealCall.noMoreExchanges$okhttp(RealCall.kt:325) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.connection.RealCall.getResponseWithInterceptorChain$okhttp(RealCall.kt:209) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.connection.RealCall$AsyncCall.run(RealCall.kt:517) ~[okhttp-4.12.0.jar:na]
	at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1095) ~[na:na]
	at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:619) ~[na:na]
	at java.base/java.lang.Thread.run(Thread.java:1447) ~[na:na]
Caused by: java.io.IOException: Canceled
	at okhttp3.internal.http.RetryAndFollowUpInterceptor.intercept(RetryAndFollowUpInterceptor.kt:72) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
	at io.opentelemetry.exporter.sender.okhttp.internal.RetryInterceptor.intercept(RetryInterceptor.java:96) ~[opentelemetry-exporter-sender-okhttp-1.49.0.jar:1.49.0]
	at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
	at okhttp3.internal.connection.RealCall.getResponseWithInterceptorChain$okhttp(RealCall.kt:201) ~[okhttp-4.12.0.jar:na]
	... 4 common frames omitted
```

```text
java.net.ConnectException: Failed to connect to localhost/127.0.0.1:4318
        at okhttp3.internal.connection.RealConnection.connectSocket(RealConnection.kt:297) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.RealConnection.connect(RealConnection.kt:207) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.ExchangeFinder.findConnection(ExchangeFinder.kt:226) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.ExchangeFinder.findHealthyConnection(ExchangeFinder.kt:106) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.ExchangeFinder.find(ExchangeFinder.kt:74) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.RealCall.initExchange$okhttp(RealCall.kt:255) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.ConnectInterceptor.intercept(ConnectInterceptor.kt:32) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.cache.CacheInterceptor.intercept(CacheInterceptor.kt:95) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.BridgeInterceptor.intercept(BridgeInterceptor.kt:83) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.RetryAndFollowUpInterceptor.intercept(RetryAndFollowUpInterceptor.kt:76) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
        at io.opentelemetry.exporter.sender.okhttp.internal.RetryInterceptor.intercept(RetryInterceptor.java:96) ~[opentelemetry-exporter-sender-okhttp-1.49.0.jar:1.49.0]
        at okhttp3.internal.http.RealInterceptorChain.proceed(RealInterceptorChain.kt:109) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.RealCall.getResponseWithInterceptorChain$okhttp(RealCall.kt:201) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.RealCall$AsyncCall.run(RealCall.kt:517) ~[okhttp-4.12.0.jar:na]
        at java.base/java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1095) ~[na:na]
        at java.base/java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:619) ~[na:na]
        at java.base/java.lang.Thread.run(Thread.java:1447) ~[na:na]
Caused by: java.net.ConnectException: Connection refused: localhost/127.0.0.1:4318
        at java.base/sun.nio.ch.Net.pollConnect(Native Method) ~[na:na]
        at java.base/sun.nio.ch.Net.pollConnectNow(Net.java:628) ~[na:na]
        at java.base/sun.nio.ch.NioSocketImpl.timedFinishConnect(NioSocketImpl.java:533) ~[na:na]
        at java.base/sun.nio.ch.NioSocketImpl.connect(NioSocketImpl.java:583) ~[na:na]
        at java.base/java.net.SocksSocketImpl.connect(SocksSocketImpl.java:282) ~[na:na]
        at java.base/java.net.Socket.connect(Socket.java:665) ~[na:na]
        at okhttp3.internal.platform.Platform.connectSocket(Platform.kt:128) ~[okhttp-4.12.0.jar:na]
        at okhttp3.internal.connection.RealConnection.connectSocket(RealConnection.kt:295) ~[okhttp-4.12.0.jar:na]
        ... 20 common frames omitted
```

## running within WSL - `mvn spring-boot:run`

opentelemetry-exporter-common-1.49.0-sources.jar

```text
WARN 206819 --- [demo] [9.0.2:31002/...] i.o.exporter.internal.http.HttpExporter  : Failed to export logs. Server responded with HTTP status code 404. Error message: Unable to parse response body, HTTP status message: Not Found
```