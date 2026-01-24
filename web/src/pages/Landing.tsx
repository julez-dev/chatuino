import Features from "@/components/Features";
import Hero from "@/components/Hero";
import Install from "@/components/Install";

export default function Landing() {
  return (
    <main id="main-content" class="flex flex-col">
      <Hero />
      <Features />
      <Install />
    </main>
  );
}
