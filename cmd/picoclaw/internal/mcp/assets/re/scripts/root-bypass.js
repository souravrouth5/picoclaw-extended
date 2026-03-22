// root-bypass.js — Bypass common root detection checks
// Covers: Build.TAGS, file existence checks, su binary, RootBeer, SafetyNet basics

Java.perform(function () {

    // --- Build.TAGS ---
    try {
        var Build = Java.use('android.os.Build');
        Build.TAGS.value = 'release-keys';
        send({ type: 'root_bypass', method: 'Build.TAGS', status: 'patched' });
    } catch (e) {}

    // --- Runtime.exec su / which su ---
    try {
        var Runtime = Java.use('java.lang.Runtime');
        Runtime.exec.overload('java.lang.String').implementation = function (cmd) {
            if (cmd.indexOf('su') !== -1 || cmd.indexOf('which') !== -1) {
                send({ type: 'root_bypass', method: 'Runtime.exec', blocked: cmd });
                throw Java.use('java.io.IOException').$new('Permission denied');
            }
            return this.exec(cmd);
        };
        Runtime.exec.overload('[Ljava.lang.String;').implementation = function (cmds) {
            for (var i = 0; i < cmds.length; i++) {
                if (cmds[i].indexOf('su') !== -1) {
                    send({ type: 'root_bypass', method: 'Runtime.exec[]', blocked: cmds[i] });
                    throw Java.use('java.io.IOException').$new('Permission denied');
                }
            }
            return this.exec(cmds);
        };
    } catch (e) {}

    // --- File existence checks for su / Magisk / Xposed ---
    var suspiciousPaths = [
        '/su', '/sbin/su', '/system/bin/su', '/system/xbin/su',
        '/data/local/xbin/su', '/data/local/bin/su',
        '/system/sd/xbin/su', '/system/bin/failsafe/su',
        '/data/local/su', '/sbin/.magisk', '/sbin/.core/mirror',
        '/data/adb/magisk', '/system/lib/libxposed_art.so'
    ];
    try {
        var File = Java.use('java.io.File');
        File.exists.implementation = function () {
            var path = this.getAbsolutePath();
            for (var i = 0; i < suspiciousPaths.length; i++) {
                if (path === suspiciousPaths[i]) {
                    send({ type: 'root_bypass', method: 'File.exists', blocked: path });
                    return false;
                }
            }
            return this.exists();
        };
    } catch (e) {}

    // --- RootBeer ---
    try {
        var RootBeer = Java.use('com.scottyab.rootbeer.RootBeer');
        RootBeer.isRooted.implementation = function () {
            send({ type: 'root_bypass', method: 'RootBeer.isRooted', status: 'bypassed' });
            return false;
        };
        RootBeer.isRootedWithoutBusyBoxCheck.implementation = function () { return false; };
    } catch (e) {}

    // --- PackageManager: hide Magisk / SuperSU ---
    try {
        var PackageManager = Java.use('android.app.ApplicationPackageManager');
        PackageManager.getPackageInfo.overload('java.lang.String', 'int').implementation = function (pkg, flags) {
            var blocked = ['com.topjohnwu.magisk', 'eu.chainfire.supersu', 'com.noshufou.android.su'];
            for (var i = 0; i < blocked.length; i++) {
                if (pkg === blocked[i]) {
                    send({ type: 'root_bypass', method: 'PackageManager', blocked: pkg });
                    throw Java.use('android.content.pm.PackageManager$NameNotFoundException').$new(pkg);
                }
            }
            return this.getPackageInfo(pkg, flags);
        };
    } catch (e) {}

    send({ type: 'root_bypass', status: 'all hooks applied' });
});
