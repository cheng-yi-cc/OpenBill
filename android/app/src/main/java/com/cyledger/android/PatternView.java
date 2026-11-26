package com.cyledger.android;
import android.animation.ValueAnimator;
import android.content.Context;
import android.graphics.*;
import android.os.SystemClock;
import android.view.*;
import java.util.*;
import java.util.function.Consumer;
final class PatternView extends View {
 private final Paint paint=new Paint(Paint.ANTI_ALIAS_FLAG);
 private final ArrayList<Integer> points=new ArrayList<>();
 private final long[] hitAt=new long[9];
 private final Consumer<String> complete;
 private final float dp;
 private float fingerX,fingerY,shakeX;
 private boolean drawing,errorState;
 private ValueAnimator errorAnimator;
 private final Runnable clearError=()->{errorState=false;points.clear();Arrays.fill(hitAt,0L);invalidate();};
 PatternView(Context c,Consumer<String> complete){super(c);this.complete=complete;this.dp=c.getResources().getDisplayMetrics().density;setMinimumHeight((int)(300*dp));setContentDescription("手势密码九宫格，连接至少四个点");}
 void showError(){
  removeCallbacks(clearError);
  if(errorAnimator!=null){errorAnimator.cancel();errorAnimator=null;}
  errorState=true;
  if(ValueAnimator.areAnimatorsEnabled()){
   errorAnimator=ValueAnimator.ofFloat(0f,-8f*dp,6f*dp,-4f*dp,2f*dp,0f);
   errorAnimator.setDuration(280);
   errorAnimator.addUpdateListener(a->{shakeX=(float)a.getAnimatedValue();invalidate();});
   errorAnimator.start();
  }
  invalidate();
  postDelayed(clearError,500);
 }
 @Override protected void onDetachedFromWindow(){
  removeCallbacks(clearError);
  if(errorAnimator!=null){errorAnimator.cancel();errorAnimator=null;}
  super.onDetachedFromWindow();
 }
 private float x(int n){return getWidth()*(1+(n%3)*2)/6f;}
 private float y(int n){return getHeight()*(1+(n/3)*2)/6f;}
 @Override protected void onDraw(Canvas canvas){
  super.onDraw(canvas);
  canvas.save();
  canvas.translate(shakeX,0f);
  int active=errorState?Color.rgb(230,87,87):Color.rgb(60,145,116);
  paint.setColor(active);paint.setStrokeWidth(4f*dp);paint.setStrokeCap(Paint.Cap.ROUND);
  for(int i=1;i<points.size();i++)canvas.drawLine(x(points.get(i-1)),y(points.get(i-1)),x(points.get(i)),y(points.get(i)),paint);
  if(drawing&&!points.isEmpty())canvas.drawLine(x(points.get(points.size()-1)),y(points.get(points.size()-1)),fingerX,fingerY,paint);
  long now=SystemClock.uptimeMillis();
  boolean animating=false;
  for(int i=0;i<9;i++){
   boolean hit=points.contains(i);
   float ease=1f;
   if(hit&&ValueAnimator.areAnimatorsEnabled()&&hitAt[i]>0L){
    long elapsed=now-hitAt[i];
    if(elapsed<140L){
     float t=Math.max(0f,elapsed/140f);
     float u=1f-t;
     ease=1f-u*u*u;
     animating=true;
    }
   }
   paint.setColor(hit?active:Color.argb(120,146,153,153));
   float dotR=hit?(6.5f+2.5f*ease)*dp:5.5f*dp;
   canvas.drawCircle(x(i),y(i),dotR,paint);
   if(hit){
    paint.setStyle(Paint.Style.STROKE);
    paint.setStrokeWidth(2f*dp);
    paint.setColor(Color.argb(Math.round(72*ease),Color.red(active),Color.green(active),Color.blue(active)));
    canvas.drawCircle(x(i),y(i),(10f+9f*ease)*dp,paint);
    paint.setStyle(Paint.Style.FILL);
   }
  }
  canvas.restore();
  if(animating)postInvalidateOnAnimation();
 }
 private void recordPoint(int idx){
  points.add(idx);
  hitAt[idx]=SystemClock.uptimeMillis();
 }
 private void hit(float x,float y){
  float radius=Math.max(24f*dp,Math.min(getWidth(),getHeight())/7.2f);
  for(int i=0;i<9;i++){
   if(Math.hypot(x-x(i),y-y(i))<radius&&!points.contains(i)){
    if(!points.isEmpty()){
     int last=points.get(points.size()-1),dx=Math.abs(last%3-i%3),dy=Math.abs(last/3-i/3);
     if((dx==2&&(dy==0||dy==2))||(dy==2&&dx==0)){
      int middle=(last+i)/2;
      if(!points.contains(middle))recordPoint(middle);
     }
    }
    recordPoint(i);
    performHapticFeedback(HapticFeedbackConstants.VIRTUAL_KEY);
    break;
   }
  }
 }
 @Override public boolean onTouchEvent(MotionEvent event){
  fingerX=event.getX();fingerY=event.getY();
  if(event.getAction()==MotionEvent.ACTION_DOWN){
   removeCallbacks(clearError);
   if(errorAnimator!=null){errorAnimator.cancel();errorAnimator=null;shakeX=0f;}
   errorState=false;points.clear();Arrays.fill(hitAt,0L);drawing=true;
  }
  if(event.getAction()==MotionEvent.ACTION_MOVE)for(int i=0;i<event.getHistorySize();i++)hit(event.getHistoricalX(i),event.getHistoricalY(i));
  hit(fingerX,fingerY);
  if(event.getAction()==MotionEvent.ACTION_UP){
   drawing=false;
   StringBuilder value=new StringBuilder();
   for(int p:points)value.append(p);
   complete.accept(value.toString());
  }
  invalidate();
  return true;
 }
}
