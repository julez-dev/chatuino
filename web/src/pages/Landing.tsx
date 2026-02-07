import { Link, Meta, Title } from "@solidjs/meta";
import Features from "@/components/Features";
import Hero from "@/components/Hero";
import Install from "@/components/Install";

export default function Landing() {
  return (
    <main id="main-content" class="flex flex-col">
      <Title>Chatuino - Terminal Twitch Chat Client</Title>
      <Meta
        name="description"
        content="A Twitch chat client that runs in your terminal. Multiple accounts, graphical emotes, 7TV, BTTV & FFZ support, message logging, and more."
      />
      <Link rel="canonical" href="https://chatuino.net/" />
      <Hero />
      <Features />
      <Install />
    </main>
  );
}
