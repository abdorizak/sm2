import Nav from "@/components/Nav";
import Hero from "@/components/Hero";
import Features from "@/components/Features";
import Quickstart from "@/components/Quickstart";
import ConfigSection from "@/components/ConfigSection";
import Architecture from "@/components/Architecture";
import CTA from "@/components/CTA";
import Footer from "@/components/Footer";

export default function Page() {
  return (
    <>
      <Nav />
      <main>
        <Hero />
        <Features />
        <Quickstart />
        <ConfigSection />
        <Architecture />
        <CTA />
      </main>
      <Footer />
    </>
  );
}
