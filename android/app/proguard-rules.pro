# gomobile 生成的绑定通过 JNI 反射调用，必须保留。
-keep class go.** { *; }
-keep class com.guyu2233.ibukirpg.mobile.** { *; }
# kotlinx.serialization
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**
-keepclassmembers @kotlinx.serialization.Serializable class com.guyu2233.ibukirpg.app.** {
    *** Companion;
    kotlinx.serialization.KSerializer serializer(...);
}
