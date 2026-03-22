// network-intercept.js — Log all HTTP/HTTPS traffic
// Covers: OkHttp3, HttpURLConnection, URL.openConnection

Java.perform(function () {

    // --- OkHttp3 RealCall.execute ---
    try {
        var RealCall = Java.use('okhttp3.internal.connection.RealCall');
        RealCall.execute.implementation = function () {
            var req = this.request();
            send({
                type: 'network',
                method: 'OkHttp3.execute',
                url: req.url().toString(),
                http_method: req.method(),
                headers: req.headers().toString()
            });
            var resp = this.execute();
            send({
                type: 'network',
                method: 'OkHttp3.response',
                url: req.url().toString(),
                code: resp.code(),
                content_type: resp.header('Content-Type')
            });
            return resp;
        };
    } catch (e) {
        send({ type: 'network', method: 'OkHttp3.RealCall', error: e.message });
    }

    // --- OkHttp3 Request.Builder (capture body) ---
    try {
        var RequestBuilder = Java.use('okhttp3.Request$Builder');
        RequestBuilder.post.implementation = function (body) {
            try {
                var Buffer = Java.use('okio.Buffer');
                var buf = Buffer.$new();
                body.writeTo(buf);
                send({
                    type: 'network',
                    method: 'OkHttp3.RequestBody',
                    body: buf.readUtf8()
                });
            } catch (e) {}
            return this.post(body);
        };
    } catch (e) {}

    // --- HttpURLConnection ---
    try {
        var HttpURLConnection = Java.use('java.net.HttpURLConnection');
        HttpURLConnection.getResponseCode.implementation = function () {
            send({
                type: 'network',
                method: 'HttpURLConnection',
                url: this.getURL().toString(),
                http_method: this.getRequestMethod()
            });
            var code = this.getResponseCode();
            send({
                type: 'network',
                method: 'HttpURLConnection.response',
                url: this.getURL().toString(),
                code: code
            });
            return code;
        };
    } catch (e) {
        send({ type: 'network', method: 'HttpURLConnection', error: e.message });
    }

    // --- URL.openConnection ---
    try {
        var URL = Java.use('java.net.URL');
        URL.openConnection.overload().implementation = function () {
            send({
                type: 'network',
                method: 'URL.openConnection',
                url: this.toString()
            });
            return this.openConnection();
        };
    } catch (e) {}

    send({ type: 'network', status: 'all hooks applied' });
});
