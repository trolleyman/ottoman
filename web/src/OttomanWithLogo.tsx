import React from "react";

export function OttomanTitle() {
  return <h1 className="text-3xl font-bold font-serif tracking-[-0.075em] text-zinc-100">
    Ottoman
  </h1>
}

type OttomanWithLogoProps = React.PropsWithChildren<{
  className?: string;
}>;

export function OttomanWithLogo({ children, className }: OttomanWithLogoProps) {
  return <>
    <div className={`flex items-center gap-4 ${className}`}>
      <picture>
        <source srcSet="/ottoman_logo.avif" type="image/avif" />
        <source srcSet="/ottoman_logo.webp" type="image/webp" />
        <img src="/ottoman_logo.png" alt="Ottoman" className="h-14 w-auto" />
      </picture>
      <div>
        <OttomanTitle />
        {children}
      </div>
    </div>
  </>
}
