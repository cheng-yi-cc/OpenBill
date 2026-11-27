package com.cyledger.android;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInstaller;
import android.os.Build;

public final class UpdateInstallReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null) return;
        int status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE);
        if (status == PackageInstaller.STATUS_PENDING_USER_ACTION) {
            Intent confirm;
            if (Build.VERSION.SDK_INT >= 33) {
                confirm = intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent.class);
            } else {
                confirm = intent.getParcelableExtra(Intent.EXTRA_INTENT);
            }
            if (confirm != null) {
                confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                try {
                    context.startActivity(confirm);
                    AppUpdater.onInstallPromptShown();
                } catch (Exception error) {
                    AppUpdater.onInstallResult(PackageInstaller.STATUS_FAILURE, "无法打开系统安装确认界面");
                }
            } else {
                AppUpdater.onInstallResult(PackageInstaller.STATUS_FAILURE, "系统未返回安装确认页面");
            }
            return;
        }
        if (status == PackageInstaller.STATUS_SUCCESS) {
            AppUpdater.onInstallResult(status, "");
            return;
        }
        String message;
        switch (status) {
            case PackageInstaller.STATUS_FAILURE_ABORTED:
                message = "已取消安装";
                break;
            case PackageInstaller.STATUS_FAILURE_CONFLICT:
                message = "安装包签名或包名与当前版本不一致，无法覆盖升级";
                break;
            case PackageInstaller.STATUS_FAILURE_INCOMPATIBLE:
                message = "安装包与当前设备或系统版本不兼容";
                break;
            case PackageInstaller.STATUS_FAILURE_STORAGE:
                message = "设备存储空间不足，请清理后重试";
                break;
            case PackageInstaller.STATUS_FAILURE_INVALID:
                message = "安装包文件无效或已损坏";
                break;
            case PackageInstaller.STATUS_FAILURE_BLOCKED:
                message = "系统阻止了安装，请检查安装权限或安全设置";
                break;
            default:
                message = "安装未完成（状态码 " + status + "）";
                break;
        }
        AppUpdater.onInstallResult(status, message);
    }
}
