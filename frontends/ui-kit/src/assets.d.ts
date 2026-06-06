// Static asset imports resolve to their served URL. Declared here so the kit
// type-checks on its own, without depending on a consumer's bundler types.
declare module '*.png' {
  const src: string;
  export default src;
}

declare module '*.svg' {
  const src: string;
  export default src;
}

declare module '*.webp' {
  const src: string;
  export default src;
}

// CSS modules import as a map of local name to generated class name.
declare module '*.module.css' {
  const classes: { readonly [key: string]: string };
  export default classes;
}
