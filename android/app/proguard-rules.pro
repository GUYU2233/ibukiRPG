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

# MediaPipe Tasks GenAI generated protobuf/AutoValue annotations are compile-time metadata.
-dontwarn com.google.auto.value.AutoValue
-dontwarn com.google.auto.value.AutoValue$Builder
-dontwarn com.google.protobuf.Internal$ProtoMethodMayReturnNull
-dontwarn com.google.protobuf.Internal$ProtoNonnullApi
-dontwarn com.google.protobuf.ProtoField
-dontwarn com.google.protobuf.ProtoPresenceBits
-dontwarn com.google.protobuf.ProtoPresenceCheckedField
