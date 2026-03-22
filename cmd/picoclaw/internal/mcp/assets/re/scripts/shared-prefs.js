// shared-prefs.js — Hook SharedPreferences read/write operations

Java.perform(function () {

    // --- SharedPreferences.getString ---
    try {
        var SharedPreferencesImpl = Java.use('android.app.SharedPreferencesImpl');
        SharedPreferencesImpl.getString.implementation = function (key, defValue) {
            var value = this.getString(key, defValue);
            send({ type: 'shared_prefs', op: 'getString', key: key, value: value });
            return value;
        };
        SharedPreferencesImpl.getInt.implementation = function (key, defValue) {
            var value = this.getInt(key, defValue);
            send({ type: 'shared_prefs', op: 'getInt', key: key, value: value });
            return value;
        };
        SharedPreferencesImpl.getBoolean.implementation = function (key, defValue) {
            var value = this.getBoolean(key, defValue);
            send({ type: 'shared_prefs', op: 'getBoolean', key: key, value: value });
            return value;
        };
    } catch (e) {
        send({ type: 'shared_prefs', method: 'SharedPreferencesImpl', error: e.message });
    }

    // --- Editor.putString (writes) ---
    try {
        var EditorImpl = Java.use('android.app.SharedPreferencesImpl$EditorImpl');
        EditorImpl.putString.implementation = function (key, value) {
            send({ type: 'shared_prefs', op: 'putString', key: key, value: value });
            return this.putString(key, value);
        };
        EditorImpl.putInt.implementation = function (key, value) {
            send({ type: 'shared_prefs', op: 'putInt', key: key, value: value });
            return this.putInt(key, value);
        };
        EditorImpl.putBoolean.implementation = function (key, value) {
            send({ type: 'shared_prefs', op: 'putBoolean', key: key, value: value });
            return this.putBoolean(key, value);
        };
    } catch (e) {
        send({ type: 'shared_prefs', method: 'EditorImpl', error: e.message });
    }

    // --- EncryptedSharedPreferences (Jetpack Security) ---
    try {
        var EncryptedSharedPreferences = Java.use('androidx.security.crypto.EncryptedSharedPreferences');
        EncryptedSharedPreferences.getString.implementation = function (key, defValue) {
            var value = this.getString(key, defValue);
            send({ type: 'shared_prefs', op: 'EncryptedSharedPreferences.getString', key: key, value: value });
            return value;
        };
    } catch (e) {}

    send({ type: 'shared_prefs', status: 'all hooks applied' });
});
