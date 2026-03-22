// intent-monitor.js — Log all Intents fired by the app

Java.perform(function () {

    function describeIntent(intent) {
        try {
            return {
                action: intent.getAction(),
                component: intent.getComponent() ? intent.getComponent().flattenToString() : null,
                data: intent.getDataString(),
                extras: intent.getExtras() ? intent.getExtras().toString() : null,
                flags: intent.getFlags()
            };
        } catch (e) { return { error: e.message }; }
    }

    // --- Context.startActivity ---
    try {
        var Activity = Java.use('android.app.Activity');
        Activity.startActivity.overload('android.content.Intent').implementation = function (intent) {
            send({ type: 'intent', op: 'startActivity', intent: describeIntent(intent) });
            return this.startActivity(intent);
        };
    } catch (e) {}

    // --- Context.startService ---
    try {
        var ContextWrapper = Java.use('android.content.ContextWrapper');
        ContextWrapper.startService.implementation = function (intent) {
            send({ type: 'intent', op: 'startService', intent: describeIntent(intent) });
            return this.startService(intent);
        };
    } catch (e) {}

    // --- Context.sendBroadcast ---
    try {
        var ContextWrapper2 = Java.use('android.content.ContextWrapper');
        ContextWrapper2.sendBroadcast.overload('android.content.Intent').implementation = function (intent) {
            send({ type: 'intent', op: 'sendBroadcast', intent: describeIntent(intent) });
            return this.sendBroadcast(intent);
        };
    } catch (e) {}

    // --- LocalBroadcastManager ---
    try {
        var LocalBroadcastManager = Java.use('androidx.localbroadcastmanager.content.LocalBroadcastManager');
        LocalBroadcastManager.sendBroadcast.implementation = function (intent) {
            send({ type: 'intent', op: 'LocalBroadcast', intent: describeIntent(intent) });
            return this.sendBroadcast(intent);
        };
    } catch (e) {}

    send({ type: 'intent', status: 'all hooks applied' });
});
