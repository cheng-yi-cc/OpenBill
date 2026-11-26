package com.cyledger.android;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.res.Configuration;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.Base64;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.webkit.CookieManager;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;
import org.json.JSONTokener;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final int PICK_FILE = 71;
    private static final int SAVE_FILE = 72;
    private static final int MAX_DOWNLOAD = 20 * 1024 * 1024;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private LinearLayout root;
    private WebView web;
    private OpenBillSplash splash;
    private boolean checkingPage;
    private String origin;
    private String pendingRoute = "";
    private ValueCallback<Uri[]> uploadCallback;
    private byte[] pendingDownload;
    private boolean loaded;
    private LocalBridge local;
    private android.webkit.GeolocationPermissions.Callback geoCallback;
    private String geoOrigin;
    private boolean destroyed;
    private Boolean appearanceDark;
    private android.window.OnBackInvokedCallback backCallback;
    private long lastBackPressAt;

    private String requestedRoute(Intent intent) {
        if (intent == null) return "";
        if (intent.getBooleanExtra("quickEntry", false)) return "/transaction/add?type=3";
        return DcaNotifier.safeRoute(intent.getStringExtra("openRoute"));
    }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        if (android.os.Build.VERSION.SDK_INT >= 31) {
            getSplashScreen().setOnExitAnimationListener(provider -> provider.remove());
        }
        root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        setContentView(root);
        boolean night = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
        appearanceDark = night;
        applyInsets();
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            backCallback = this::onBackPressed;
            getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT, backCallback);
        }
        try {
            MaintenanceActivity.recover(getFilesDir());
        } catch (Exception error) {
            showStatus("上次备份恢复未完成，你的账本文件仍保存在手机中，暂未丢失。请点击下方按钮重新启动。", true, true);
            return;
        }
        local=new LocalBridge(this);
        origin = LedgerRuntime.origin(this);
        pendingRoute = requestedRoute(getIntent());
        DcaNotifier.ensureChannel(this);
        DcaScheduler.ensurePeriodic(this);
        root.setBackgroundColor(night ? Color.rgb(20, 33, 39) : Color.rgb(247, 244, 235));
        splash = new OpenBillSplash(this);
        root.addView(splash, new LinearLayout.LayoutParams(-1, -1));
        LedgerRuntime.ensureStarted(this);
        io.execute(() -> waitForBackend(state));
    }

    private void applyInsets() {
        if (android.os.Build.VERSION.SDK_INT >= 30) {
            getWindow().setDecorFitsSystemWindows(false);
            root.setOnApplyWindowInsetsListener((view, insets) -> {
                android.graphics.Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                android.graphics.Insets keyboard = insets.getInsets(WindowInsets.Type.ime());
                view.setPadding(bars.left, bars.top, bars.right, Math.max(bars.bottom, keyboard.bottom));
                return WindowInsets.CONSUMED;
            });
            WindowInsetsController controller = getWindow().getInsetsController();
            if (controller != null) {
                boolean dark = appearanceDark != null ? appearanceDark : (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
                int flags = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                controller.setSystemBarsAppearance(dark ? 0 : flags, flags);
            }
        }
        root.requestApplyInsets();
    }

    private void showStatus(String message, boolean retry) {
        showStatus(message, retry, LedgerRuntime.getStartupError() != null);
    }

    private void showStatus(String message, boolean retry, boolean restartProcess) {
        splash = null;
        checkingPage = false;
        root.removeAllViews();
        boolean dark = appearanceDark != null ? appearanceDark : (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
        float dp = getResources().getDisplayMetrics().density;
        int pad = Math.round(24 * dp);
        root.setBackgroundColor(dark ? Color.rgb(20, 33, 39) : Color.rgb(247, 244, 237));
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setGravity(Gravity.CENTER);
        box.setPadding(pad, pad, pad, pad);
        TextView label = new TextView(this);
        label.setText(message);
        label.setTextColor(dark ? Color.rgb(227, 236, 233) : Color.rgb(40, 53, 49));
        label.setTextSize(16);
        label.setLineSpacing(0, 1.45f);
        label.setGravity(Gravity.CENTER);
        box.addView(label, new LinearLayout.LayoutParams(-1, -2));
        if (retry) {
            Button button = new Button(this);
            button.setText(restartProcess ? "重新启动 OpenBill" : "重新连接手机账本");
            LinearLayout.LayoutParams btnParams = new LinearLayout.LayoutParams(-1, -2);
            btnParams.topMargin = Math.round(20 * dp);
            button.setOnClickListener(view -> {
                if (restartProcess) restartApp();
                else io.execute(() -> waitForBackend(null));
            });
            box.addView(button, btnParams);
        }
        root.addView(box, new LinearLayout.LayoutParams(-1, -1));
    }

    private void restartApp() {
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch != null) {
            launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
            startActivity(launch);
        }
        finish();
        android.os.Process.killProcess(android.os.Process.myPid());
    }

    private void waitForBackend(Bundle state) {
        for (int attempt = 0; attempt < 180 && !destroyed; attempt++) {
            String err = LedgerRuntime.getStartupError();
            if (err != null) {
                handler.post(() -> { if (!destroyed) showStatus(err, true); });
                return;
            }
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL(origin + "/healthz.json").openConnection();
                connection.setConnectTimeout(500);
                connection.setReadTimeout(1000);
                if (connection.getResponseCode() == 200) {
                    handler.post(() -> { if (!destroyed) openLedger(state); });
                    return;
                }
            } catch (Exception ignored) {
                // Native startup can take a few seconds on the first installation.
            } finally {
                if (connection != null) connection.disconnect();
            }
            try { Thread.sleep(500); } catch (InterruptedException error) { return; }
        }
        handler.post(() -> { if (!destroyed) showStatus("账本暂时没有响应。数据仍保存在手机，请关闭后重新打开。", true); });
    }

    private void openLedger(Bundle state) { local.ensureUnlocked(()->openLedgerUnlocked(state)); }
    private void openLedgerUnlocked(Bundle state) {
        loaded = true;
        root.removeAllViews();
        if (web == null) {
            web = new WebView(this);
            local.attach(web);
            web.addJavascriptInterface(local,"CYLedgerLocal");
            web.addJavascriptInterface(new ReminderBridge(this, web), "CYLedgerReminders");
            web.addJavascriptInterface(new AppearanceBridge(), "CYLedgerAppearance");
            WebView.setWebContentsDebuggingEnabled((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
            WebSettings settings = web.getSettings();
            settings.setJavaScriptEnabled(true);
            settings.setDomStorageEnabled(true);
            settings.setCacheMode(WebSettings.LOAD_NO_CACHE);
            settings.setAllowFileAccess(false);
            settings.setAllowContentAccess(true);
            settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
            settings.setMediaPlaybackRequiresUserGesture(true);
            CookieManager.getInstance().setAcceptThirdPartyCookies(web, false);
            web.setWebViewClient(new WebViewClient() {
                @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                    if (!isLocal(request.getUrl().toString())) return new WebResourceResponse("text/plain","UTF-8",403,"Forbidden",Collections.emptyMap(),new ByteArrayInputStream(new byte[0]));
                    // Credentials enter only this app's WebView, through JNI.
                    // No HTTP session endpoint or public JavaScript bridge exists.
                    if (!request.isForMainFrame() || !isLocal(request.getUrl().toString())
                        || !"/personal".equals(request.getUrl().getPath())) return null;
                    try (InputStream stream = new FileInputStream(new File(getFilesDir(), "ledger/public/mobile.html"))) {
                        // Refresh after every reload: the Vue token refresh can
                        // revoke a previously injected startup token.
                        String personalSession = NativeBridge.session();
                        if (personalSession == null) throw new Exception("personal owner unavailable");
                        String html = new String(LedgerRuntime.readBounded(stream, 1024 * 1024), StandardCharsets.UTF_8);
                        String script = "<script>window.__CYLEDGER_PERSONAL__=true;"
                            + "(()=>{const s=JSON.parse(" + JSONObject.quote(personalSession) + ");"
                            + "localStorage.setItem('ebk_user_token',s.token);"
                            + "localStorage.setItem('ebk_user_info',JSON.stringify(s.user));"
                            + "if(location.hash.includes('/login')||location.hash.includes('/signup'))history.replaceState(null,'','/personal');"
                            + "})();</script>";
                        File restored=new File(getFilesDir(),"ledger/settings.json");
                        if(restored.isFile()){String settings=new String(java.nio.file.Files.readAllBytes(restored.toPath()),StandardCharsets.UTF_8);new JSONObject(settings);script+="<script>(()=>{let settings=JSON.parse("+JSONObject.quote(settings).replace("<","\\u003c").replace(">","\\u003e")+");Object.keys(localStorage).filter(k=>k==='ebk_app_settings'||k.startsWith('cy_ledger_experience_')).forEach(k=>localStorage.removeItem(k));Object.entries(settings).forEach(([k,v])=>localStorage.setItem(k,v));window.addEventListener('load',()=>CYLedgerLocal.action('settingsRestored','{}'));})();</script>";}
                        html = html.replace("<head>", "<head>" + script);
                        return new WebResourceResponse("text/html", "UTF-8", 200, "OK",
                            Collections.singletonMap("Cache-Control", "no-store"),
                            new ByteArrayInputStream(html.getBytes(StandardCharsets.UTF_8)));
                    } catch (Exception error) {
                        return new WebResourceResponse("text/plain", "UTF-8", 503, "Unavailable",
                            Collections.singletonMap("Cache-Control", "no-store"),
                            new ByteArrayInputStream("个人账本页面加载失败，请关闭后重试。".getBytes(StandardCharsets.UTF_8)));
                    }
                }
                @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                    if (isLocal(request.getUrl().toString())) return false;
                    if (!request.isForMainFrame()) return true;
                    Uri uri = request.getUrl();
                    if ("https".equals(uri.getScheme()) || "http".equals(uri.getScheme())) {
                        String host = uri.getHost() != null ? uri.getHost() : uri.toString();
                        new AlertDialog.Builder(MainActivity.this)
                            .setTitle("使用浏览器打开链接？")
                            .setMessage(host + "\n" + uri.toString())
                            .setPositiveButton("打开", (dialog, which) -> {
                                try { startActivity(new Intent(Intent.ACTION_VIEW, uri)); }
                                catch (Exception error) { Toast.makeText(MainActivity.this, "没有可用的浏览器", Toast.LENGTH_SHORT).show(); }
                            }).setNegativeButton("取消", null).show();
                    }
                    return true;
                }
                @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                    if (request.isForMainFrame() && isLocal(request.getUrl().toString())) {
                        showStatus("手机账本页面加载失败，请重试。", true);
                    }
                }
                @Override public void onPageFinished(WebView view, String url) {
                    if (isLocal(url)) {
                        root.requestApplyInsets();
                        if (splash != null && !checkingPage) {
                            checkingPage = true;
                            revealLedger(0);
                        }
                    }
                }
                @Override public void onReceivedHttpError(WebView view, WebResourceRequest request, WebResourceResponse response) {
                    if (request.isForMainFrame() && isLocal(request.getUrl().toString())) {
                        showStatus("手机账本页面加载失败，请重试。", true);
                    }
                }
            });
            web.setWebChromeClient(new WebChromeClient() {
                @Override public void onGeolocationPermissionsShowPrompt(String pageOrigin,android.webkit.GeolocationPermissions.Callback callback){if(!isLocal(pageOrigin)){callback.invoke(pageOrigin,false,false);return;}geoOrigin=pageOrigin;geoCallback=callback;if(checkSelfPermission(android.Manifest.permission.ACCESS_FINE_LOCATION)==android.content.pm.PackageManager.PERMISSION_GRANTED||checkSelfPermission(android.Manifest.permission.ACCESS_COARSE_LOCATION)==android.content.pm.PackageManager.PERMISSION_GRANTED){callback.invoke(pageOrigin,true,false);geoCallback=null;}else requestPermissions(new String[]{android.Manifest.permission.ACCESS_FINE_LOCATION,android.Manifest.permission.ACCESS_COARSE_LOCATION},804);}

                @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams parameters) {
                    if (uploadCallback != null) uploadCallback.onReceiveValue(null);
                    uploadCallback = callback;
                    Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
                    intent.addCategory(Intent.CATEGORY_OPENABLE);
                    String[] types = parameters.getAcceptTypes();
                    for(int i=0;i<types.length;i++){if(types[i].startsWith(".")){String mime=android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(types[i].substring(1));types[i]=mime==null?"*/*":mime;}}
                    intent.setType(types.length == 1 && !types[0].isEmpty() ? types[0] : "*/*");
                    if (types.length > 1) intent.putExtra(Intent.EXTRA_MIME_TYPES, types);
                    intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, parameters.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE);
                    try { startActivityForResult(intent, PICK_FILE); }
                    catch (Exception error) { callback.onReceiveValue(null); uploadCallback = null; }
                    return true;
                }
            });
            web.setDownloadListener((url, userAgent, disposition, mime, size) -> {
                if (pendingDownload != null) return;
                String filename = android.webkit.URLUtil.guessFileName(url, disposition, mime);
                if (url.startsWith("blob:" + origin + "/")) {
                    // Preserve the export filename supplied by the existing Vue page.
                    String expression = "(function(){let a=Array.from(document.querySelectorAll('a[download]')).find(a=>a.href==="
                        + JSONObject.quote(url) + ");return a?a.download:'';})()";
                    web.evaluateJavascript(expression, value -> {
                        String name = filename;
                        try {
                            Object decoded = new JSONTokener(value).nextValue();
                            if (decoded instanceof String) {
                                String suggested = ((String) decoded).trim();
                                if (!suggested.isEmpty() && suggested.length() < 180 && !suggested.contains("/") && !suggested.contains("\\")) name = suggested;
                            }
                        } catch (Exception ignored) {}
                        readBlob(url, name, mime);
                    });
                }
                else if (isLocal(url)) io.execute(() -> download(url, userAgent, filename, mime));
            });
        }
        if (web.getParent() instanceof ViewGroup) ((ViewGroup) web.getParent()).removeView(web);
        FrameLayout stage = new FrameLayout(this);
        stage.addView(web, new FrameLayout.LayoutParams(-1, -1));
        if (splash == null) splash = new OpenBillSplash(this);
        stage.addView(splash, new FrameLayout.LayoutParams(-1, -1));
        root.addView(stage, new LinearLayout.LayoutParams(-1, -1));
        if (state != null && web.restoreState(state) != null) {
            if (!pendingRoute.isEmpty()) {
                String route = pendingRoute;
                pendingRoute = "";
                handler.post(() -> navigateTo(route));
            }
            return;
        }
        String initialRoute = pendingRoute;
        pendingRoute = "";
        web.loadUrl(origin + "/personal" + (initialRoute.isEmpty() ? "" : "#!" + initialRoute));
    }

    private void navigateTo(String route) {
        if (web == null || route == null || route.isEmpty()) return;
        web.evaluateJavascript("document.querySelector('.view-main')?.f7View?.router.navigate(" + JSONObject.quote(route) + ")", null);
    }

    boolean isLocalPage(String url) { return url!=null&&isLocal(url); }
    private boolean isLocal(String url) {
        Uri uri = Uri.parse(url);
        Uri local = Uri.parse(origin);
        return "http".equals(uri.getScheme()) && "127.0.0.1".equals(uri.getHost()) && uri.getPort() == local.getPort();
    }

    // Wait for the actual Framework7 page, rather than only the HTML load event.
    private void revealLedger(int attempt) {
        if (destroyed || web == null || splash == null) return;
        web.evaluateJavascript("!!document.querySelector('.view-main .page-current')", value -> {
            if (destroyed || splash == null) return;
            if ("true".equals(value)) {
                OpenBillSplash finished = splash;
                splash = null;
                checkingPage = false;
                if (!pendingRoute.isEmpty()) {
                    String route = pendingRoute;
                    pendingRoute = "";
                    navigateTo(route);
                }
                Runnable remove = () -> {
                    if (finished.getParent() instanceof ViewGroup) ((ViewGroup) finished.getParent()).removeView(finished);
                };
                if (android.animation.ValueAnimator.areAnimatorsEnabled()) {
                    finished.animate()
                        .alpha(0f)
                        .scaleX(1.025f)
                        .scaleY(1.025f)
                        .setDuration(220)
                        .setInterpolator(new android.view.animation.PathInterpolator(0.22f, 1f, 0.36f, 1f))
                        .withEndAction(remove)
                        .start();
                } else {
                    remove.run();
                }
            } else if (attempt < 200) handler.postDelayed(() -> revealLedger(attempt + 1), 100);
            else showStatus("账本界面未能打开，请重试。", true);
        });
    }

    private final class AppearanceBridge {
        @android.webkit.JavascriptInterface public void setTheme(String color, boolean dark) {
            if (color == null || !color.matches("#[0-9a-fA-F]{6}")) return;
            handler.post(() -> {
                if (destroyed || web == null || !isLocal(web.getUrl())) return;
                int value = Color.parseColor(color);
                appearanceDark = dark;
                root.setBackgroundColor(value);
                getWindow().setStatusBarColor(value);
                getWindow().setNavigationBarColor(value);
                if (android.os.Build.VERSION.SDK_INT >= 30) {
                    WindowInsetsController controller=getWindow().getInsetsController();
                    int flags=WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS|WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                    if(controller!=null)controller.setSystemBarsAppearance(dark?0:flags,flags);
                }
            });
        }
    }

    private void download(String url, String userAgent, String name, String mime) {
        HttpURLConnection connection = null;
        try {
            connection = (HttpURLConnection) new URL(url).openConnection();
            connection.setInstanceFollowRedirects(false);
            connection.setConnectTimeout(10000);
            connection.setReadTimeout(60000);
            connection.setRequestProperty("User-Agent", userAgent);
            String cookies = CookieManager.getInstance().getCookie(url);
            if (cookies != null) connection.setRequestProperty("Cookie", cookies);
            if (connection.getResponseCode() != 200) throw new Exception("download failed");
            byte[] data;
            try (InputStream stream = connection.getInputStream()) { data = LedgerRuntime.readBounded(stream, MAX_DOWNLOAD); }
            handler.post(() -> saveDownload(data, name, mime));
        } catch (Exception error) {
            String msg = error.getMessage() != null && error.getMessage().contains("large")
                ? "导出文件超过 20 MB，请缩小日期范围分批导出"
                : "导出失败，请重试";
            handler.post(() -> Toast.makeText(this, msg, Toast.LENGTH_LONG).show());
        } finally { if (connection != null) connection.disconnect(); }
    }

    private void readBlob(String url, String name, String mime) {
        String script = "window.__cyledgerDownload=null;fetch(" + JSONObject.quote(url) + ")"
            + ".then(r=>r.blob()).then(b=>{if(b.size>" + MAX_DOWNLOAD + "){window.__cyledgerDownload={error:'large'};return;}"
            + "let r=new FileReader();r.onload=()=>window.__cyledgerDownload={data:r.result.split(',')[1],mime:b.type};"
            + "r.onerror=()=>window.__cyledgerDownload={error:'read'};r.readAsDataURL(b)})"
            + ".catch(()=>window.__cyledgerDownload={error:'fetch'});";
        web.evaluateJavascript(script, result -> pollBlob(name, mime, 0));
    }

    private void pollBlob(String name, String mime, int attempt) {
        if (destroyed) return;
        if (attempt >= 300) {
            Toast.makeText(this, "导出准备超时，请重试", Toast.LENGTH_LONG).show();
            return;
        }
        web.evaluateJavascript("JSON.stringify(window.__cyledgerDownload)", value -> {
            try {
                Object raw = new JSONTokener(value).nextValue();
                if (!(raw instanceof String) || "null".equals(raw)) {
                    handler.postDelayed(() -> pollBlob(name, mime, attempt + 1), 100);
                    return;
                }
                JSONObject result = new JSONObject((String) raw);
                web.evaluateJavascript("delete window.__cyledgerDownload", null);
                String errKind = result.optString("error", "");
                if (!errKind.isEmpty() || result.optBoolean("error", false)) {
                    String msg = "large".equals(errKind) ? "导出文件超过 20 MB，请缩小日期范围分批导出" : "导出失败，请重试";
                    Toast.makeText(this, msg, Toast.LENGTH_LONG).show();
                    return;
                }
                saveDownload(Base64.decode(result.getString("data"), Base64.DEFAULT), name, result.optString("mime", mime));
            } catch (Exception error) { Toast.makeText(this, "导出失败，请重试", Toast.LENGTH_LONG).show(); }
        });
    }

    private void saveDownload(byte[] data, String name, String mime) {
        if (destroyed) return;
        if (data.length > MAX_DOWNLOAD) {
            Toast.makeText(this, "导出文件超过 20 MB，请缩小日期范围分批导出", Toast.LENGTH_LONG).show();
            return;
        }
        pendingDownload = data;
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        String mimeType = mime == null || mime.isEmpty() ? "application/octet-stream" : mime.split(";", 2)[0].trim();
        intent.setType(mimeType);
        intent.putExtra(Intent.EXTRA_TITLE, name);
        try { startActivityForResult(intent, SAVE_FILE); }
        catch (Exception error) { pendingDownload = null; Toast.makeText(this, "无法打开文件保存位置", Toast.LENGTH_LONG).show(); }
    }

    @Override protected void onActivityResult(int requestCode, int resultCode, Intent result) {
        super.onActivityResult(requestCode, resultCode, result);
        local.activityResult(requestCode,resultCode,result);
        if (requestCode == PICK_FILE && uploadCallback != null) {
            Uri[] files=null; if(resultCode==RESULT_OK && result!=null){android.content.ClipData clip=result.getClipData();if(clip!=null){files=new Uri[clip.getItemCount()];for(int i=0;i<files.length;i++)files[i]=clip.getItemAt(i).getUri();}else if(result.getData()!=null)files=new Uri[]{result.getData()};}
            local.selectedFiles(files);
            uploadCallback.onReceiveValue(files);
            uploadCallback = null;
        }
        if (requestCode == SAVE_FILE && pendingDownload != null) {
            byte[] data = pendingDownload;
            pendingDownload = null;
            if (resultCode == RESULT_OK && result != null && result.getData() != null) {
                Uri uri = result.getData();
                io.execute(() -> {
                    try (OutputStream output = getContentResolver().openOutputStream(uri)) {
                        if (output == null) throw new Exception("output unavailable");
                        output.write(data);
                        handler.post(() -> Toast.makeText(this, "文件已保存", Toast.LENGTH_SHORT).show());
                    } catch (Exception error) { handler.post(() -> Toast.makeText(this, "文件保存失败", Toast.LENGTH_LONG).show()); }
                });
            }
        }
    }

    @Override public void onBackPressed() {
        if (loaded && web != null) {
            web.evaluateJavascript("(function(){return !!(window.__openbillHandleBack && window.__openbillHandleBack());})()", result -> {
                if (!"true".equals(result)) {
                    exitLedger();
                }
            });
        } else exitLedger();
    }

    private void exitLedger() {
        if (local != null && local.exitConfirm()) {
            long now = android.os.SystemClock.elapsedRealtime();
            if (now - lastBackPressAt <= 2000L) {
                lastBackPressAt = 0;
                moveTaskToBack(true);
            } else {
                lastBackPressAt = now;
                Toast.makeText(this, "再按一次退出 OpenBill", Toast.LENGTH_SHORT).show();
            }
        } else {
            moveTaskToBack(true);
        }
    }
    @Override protected void onStop(){local.stopped();DcaScheduler.scheduleImmediate(getApplicationContext(),false);super.onStop();}
    @Override protected void onResume(){super.onResume();if(local!=null)local.resumed();if(web!=null&&isLocal(web.getUrl()))web.evaluateJavascript("window.dispatchEvent(new CustomEvent('cyledger-notification-permission'))",null);}
    @Override protected void onNewIntent(Intent intent){super.onNewIntent(intent);setIntent(intent);String route=requestedRoute(intent);if(!route.isEmpty()){if(web!=null&&loaded)local.ensureUnlocked(()->navigateTo(route));else pendingRoute=route;}}
    @Override public void onRequestPermissionsResult(int request,String[] permissions,int[] results){super.onRequestPermissionsResult(request,permissions,results);if(request==701&&web!=null&&isLocal(web.getUrl())){web.evaluateJavascript("window.dispatchEvent(new CustomEvent('cyledger-notification-permission'))",null);}if(request==804&&geoCallback!=null){boolean granted=false;for(int result:results)if(result==android.content.pm.PackageManager.PERMISSION_GRANTED)granted=true;geoCallback.invoke(geoOrigin,granted,false);geoCallback=null;}}
    @Override public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        applyInsets();
    }

    @Override protected void onSaveInstanceState(Bundle state) {
        if (web != null) web.saveState(state);
        super.onSaveInstanceState(state);
    }

    @Override protected void onDestroy() {
        destroyed = true;
        if (android.os.Build.VERSION.SDK_INT >= 33 && backCallback != null) {
            getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(backCallback);
        }
        handler.removeCallbacksAndMessages(null);
        if (uploadCallback != null) uploadCallback.onReceiveValue(null);
        if (web != null) {
            if (web.getParent() instanceof ViewGroup) ((ViewGroup) web.getParent()).removeView(web);
            web.destroy();
        }
        io.shutdownNow();
        if(local!=null)local.destroy();
        super.onDestroy();
    }
}
