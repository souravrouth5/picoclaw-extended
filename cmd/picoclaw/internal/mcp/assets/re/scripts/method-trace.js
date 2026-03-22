// method-trace.js — Trace method calls on a target class
// Send a message to configure: {"type":"config","className":"com.example.Foo","methodName":"*"}
// methodName "*" traces all methods on the class.

var targetClass = null;
var targetMethod = null;

recv('config', function (msg) {
    targetClass = msg.className || null;
    targetMethod = msg.methodName || '*';
    if (targetClass) {
        attachTrace(targetClass, targetMethod);
    }
});

function attachTrace(className, methodName) {
    Java.perform(function () {
        try {
            var Clazz = Java.use(className);
            var methods = Clazz.class.getDeclaredMethods();

            methods.forEach(function (method) {
                var name = method.getName();
                if (methodName !== '*' && name !== methodName) return;

                try {
                    Clazz[name].overloads.forEach(function (overload) {
                        overload.implementation = function () {
                            var args = Array.prototype.slice.call(arguments).map(function (a) {
                                try { return JSON.stringify(a); } catch (e) { return String(a); }
                            });
                            send({
                                type: 'method_trace',
                                class: className,
                                method: name,
                                args: args,
                                thread: Java.use('java.lang.Thread').currentThread().getName()
                            });
                            var ret = this[name].apply(this, arguments);
                            send({
                                type: 'method_trace_return',
                                class: className,
                                method: name,
                                return: String(ret)
                            });
                            return ret;
                        };
                    });
                } catch (e) {}
            });

            send({ type: 'method_trace', status: 'hooked', class: className, method: methodName });
        } catch (e) {
            send({ type: 'method_trace', error: e.message, class: className });
        }
    });
}

// Default: trace if className was baked in at injection time via rpc.exports
rpc.exports = {
    trace: function (className, methodName) {
        attachTrace(className, methodName || '*');
    }
};
