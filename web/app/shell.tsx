"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";

const links = [
  { href: "/", label: "Overview" },
  { href: "/links", label: "Links" },
  { href: "/analytics", label: "Analytics" },
];

export function Shell({ children }: { children: React.ReactNode }) {
  const auth = useAuth();
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  useEffect(() => setOpen(false), [pathname]);
  const active = (href: string) => href === "/" ? pathname === "/" : pathname.startsWith(href);

  return (
    <>
      <aside className={open ? "sidebar open" : "sidebar"}>
        <div className="sidebar-top">
          <Link href="/" className="brand" aria-label="Uzip overview">Uzip<span>.</span></Link>
          <button className="menu-button" onClick={() => setOpen(value => !value)} aria-expanded={open} aria-label="Toggle navigation">
            <span/><span/><span/>
          </button>
        </div>
        <nav aria-label="Main navigation">
          {links.map(link => <Link key={link.href} href={link.href} className={active(link.href) ? "active" : ""}>{link.label}</Link>)}
        </nav>
        <div className="tenant">
          <div><strong>{auth.authenticated ? auth.claims?.email ?? "Demo Organization" : "Signed out"}</strong><br/><small>{auth.authenticated ? auth.claims?.["custom:role"] ?? "Owner" : "No active session"}</small></div>
          {auth.authenticated && <button className="link-button" onClick={auth.logout}>Sign out</button>}
        </div>
      </aside>
      {open && <button className="backdrop" onClick={() => setOpen(false)} aria-label="Close navigation"/>}
      <main>{children}</main>
    </>
  );
}
