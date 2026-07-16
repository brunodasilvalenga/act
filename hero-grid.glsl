/** @resolution */
uniform vec2 u_resolution;

/**
 * @label Dot Color
 * @color
 * @default #39E75F
 */
uniform vec3 u_color;

/**
 * @label Spacing
 * @default 26
 */
uniform float u_spacing;

void main() {
  vec2 coord = mod(gl_FragCoord.xy, u_spacing);
  vec2 center = vec2(u_spacing * 0.5);
  float dist = length(coord - center);
  float dot = 1.0 - smoothstep(0.0, 1.4, dist);
  gl_FragColor = vec4(u_color, dot * 0.16);
}
