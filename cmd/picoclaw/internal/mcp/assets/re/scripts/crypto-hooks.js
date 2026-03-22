// crypto-hooks.js — Hook javax.crypto operations, log keys and plaintext

Java.perform(function () {

    function bytesToHex(bytes) {
        if (!bytes) return 'null';
        try {
            var arr = Java.array('byte', bytes);
            var hex = '';
            for (var i = 0; i < arr.length; i++) {
                hex += ('0' + (arr[i] & 0xff).toString(16)).slice(-2);
            }
            return hex;
        } catch (e) { return String(bytes); }
    }

    function bytesToString(bytes) {
        try {
            return Java.use('java.lang.String').$new(bytes, 'UTF-8');
        } catch (e) { return bytesToHex(bytes); }
    }

    // --- Cipher.init: capture algorithm + key ---
    try {
        var Cipher = Java.use('javax.crypto.Cipher');
        Cipher.init.overload('int', 'java.security.Key').implementation = function (mode, key) {
            send({
                type: 'crypto',
                op: 'Cipher.init',
                algorithm: this.getAlgorithm(),
                mode: mode === 1 ? 'ENCRYPT' : 'DECRYPT',
                key_hex: bytesToHex(key.getEncoded()),
                key_algorithm: key.getAlgorithm()
            });
            return this.init(mode, key);
        };
        Cipher.init.overload('int', 'java.security.Key', 'java.security.spec.AlgorithmParameterSpec').implementation = function (mode, key, params) {
            var ivHex = '';
            try {
                var IvSpec = Java.use('javax.crypto.spec.IvParameterSpec');
                var ivSpec = Java.cast(params, IvSpec);
                ivHex = bytesToHex(ivSpec.getIV());
            } catch (e) {}
            send({
                type: 'crypto',
                op: 'Cipher.init+IV',
                algorithm: this.getAlgorithm(),
                mode: mode === 1 ? 'ENCRYPT' : 'DECRYPT',
                key_hex: bytesToHex(key.getEncoded()),
                iv_hex: ivHex
            });
            return this.init(mode, key, params);
        };
    } catch (e) {
        send({ type: 'crypto', error: 'Cipher.init hook failed: ' + e.message });
    }

    // --- Cipher.doFinal: capture plaintext/ciphertext ---
    try {
        var Cipher2 = Java.use('javax.crypto.Cipher');
        Cipher2.doFinal.overload('[B').implementation = function (input) {
            var result = this.doFinal(input);
            send({
                type: 'crypto',
                op: 'Cipher.doFinal',
                algorithm: this.getAlgorithm(),
                input_hex: bytesToHex(input),
                input_str: bytesToString(input),
                output_hex: bytesToHex(result)
            });
            return result;
        };
    } catch (e) {}

    // --- MessageDigest: log hash inputs ---
    try {
        var MessageDigest = Java.use('java.security.MessageDigest');
        MessageDigest.digest.overload('[B').implementation = function (input) {
            var result = this.digest(input);
            send({
                type: 'crypto',
                op: 'MessageDigest.digest',
                algorithm: this.getAlgorithm(),
                input_str: bytesToString(input),
                output_hex: bytesToHex(result)
            });
            return result;
        };
    } catch (e) {}

    // --- Mac (HMAC): log key + input ---
    try {
        var Mac = Java.use('javax.crypto.Mac');
        Mac.doFinal.overload('[B').implementation = function (input) {
            var result = this.doFinal(input);
            send({
                type: 'crypto',
                op: 'Mac.doFinal',
                algorithm: this.getAlgorithm(),
                input_str: bytesToString(input),
                output_hex: bytesToHex(result)
            });
            return result;
        };
    } catch (e) {}

    // --- SecretKeySpec: log raw key material ---
    try {
        var SecretKeySpec = Java.use('javax.crypto.spec.SecretKeySpec');
        SecretKeySpec.$init.overload('[B', 'java.lang.String').implementation = function (key, alg) {
            send({
                type: 'crypto',
                op: 'SecretKeySpec',
                algorithm: alg,
                key_hex: bytesToHex(key),
                key_len: key.length
            });
            return this.$init(key, alg);
        };
    } catch (e) {}

    send({ type: 'crypto', status: 'all hooks applied' });
});
