// ssl-unpin.js — Bypass SSL certificate pinning
// Covers: OkHttp3, TrustManager, X509TrustManager, Conscrypt, WebViewClient

Java.perform(function () {

    // --- TrustManager: accept all certs ---
    try {
        var TrustManager = Java.registerClass({
            name: 'com.re.TrustManager',
            implements: [Java.use('javax.net.ssl.X509TrustManager')],
            methods: {
                checkClientTrusted: function (chain, authType) {},
                checkServerTrusted: function (chain, authType) {},
                getAcceptedIssuers: function () { return []; }
            }
        });
        var SSLContext = Java.use('javax.net.ssl.SSLContext');
        var ctx = SSLContext.getInstance('TLS');
        ctx.init(null, [TrustManager.$new()], null);
        SSLContext.getDefault.implementation = function () { return ctx; };
        send({ type: 'ssl_unpin', method: 'TrustManager', status: 'hooked' });
    } catch (e) {
        send({ type: 'ssl_unpin', method: 'TrustManager', error: e.message });
    }

    // --- OkHttp3 CertificatePinner ---
    try {
        var CertificatePinner = Java.use('okhttp3.CertificatePinner');
        CertificatePinner.check.overload('java.lang.String', 'java.util.List').implementation = function (h, c) {
            send({ type: 'ssl_unpin', method: 'OkHttp3.CertificatePinner', host: h });
        };
        CertificatePinner.check.overload('java.lang.String', 'java.security.cert.Certificate').implementation = function (h, c) {
            send({ type: 'ssl_unpin', method: 'OkHttp3.CertificatePinner.cert', host: h });
        };
        send({ type: 'ssl_unpin', method: 'OkHttp3', status: 'hooked' });
    } catch (e) {
        send({ type: 'ssl_unpin', method: 'OkHttp3', error: e.message });
    }

    // --- OkHttp3 HostnameVerifier ---
    try {
        var OkHostnameVerifier = Java.use('okhttp3.internal.tls.OkHostnameVerifier');
        OkHostnameVerifier.verify.overload('java.lang.String', 'javax.net.ssl.SSLSession').implementation = function (h, s) {
            return true;
        };
    } catch (e) {}

    // --- Conscrypt TrustManagerImpl ---
    try {
        var TrustManagerImpl = Java.use('com.android.org.conscrypt.TrustManagerImpl');
        TrustManagerImpl.verifyChain.implementation = function (chain, ocspData, tlsSctData, host, clientAuth, algorithms) {
            send({ type: 'ssl_unpin', method: 'Conscrypt', host: host });
            return chain;
        };
    } catch (e) {
        send({ type: 'ssl_unpin', method: 'Conscrypt', error: e.message });
    }

    // --- WebViewClient SSL errors ---
    try {
        var WebViewClient = Java.use('android.webkit.WebViewClient');
        WebViewClient.onReceivedSslError.implementation = function (view, handler, error) {
            handler.proceed();
            send({ type: 'ssl_unpin', method: 'WebViewClient', status: 'proceeded' });
        };
    } catch (e) {}

    send({ type: 'ssl_unpin', status: 'all hooks applied' });
});
