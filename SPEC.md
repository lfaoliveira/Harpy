# Specification file (structured towards Humans and coding Agents alike)

This file describes what Harpy is, what is isn't and general rules towards the project.

## 1. What Harpy is.

Harpy is a Universal App Manager or All-In-One App Manager, similar to Software Stores like Google Play Store, App Store, Microsoft Store, etc.

A Universal App Manager is an App that managers many different apps, sometimes on different machines than the app itself is installed. (e.g: a Windows app that managers apps installed into the users Linux Machine.).

The whole goal of harpy is to be an easy-to-use app that manages all installs for you, seamslessly and with CLI support (built to be CLI-friendly from the start).

A normal user and a software developer need both to be able to use Harpy seamlessly and without major complaints, aiming to be as robust as possible to failures, OS variations, unwanted behaviours and overall instability that might make the software dislikable or unusable.

For developers, aditional features, such as IDE setting sync and management, .env management, etc are welcome to be added to the app.

## 2. What Harpy is not

Harpy is not:

- A package manager, meaning software that manages packages such as mise, mpm, npm, uv, pip, cargo, etc.
- An Integrated Development Environment (IDE), meaning that it does not try to be an advanced or even simplistic code editor.
- A security app: Users are completely responsible for any install made through the app. Harpy and it's team do not take legal responsability over hack attempts made because of apps installed through the app. Harpy does allow installs from unofficial sources such as what is wrongly but commonly called "sideloading" in mobile devices or installs made outside Microsoft Store, but nevertheless those are the end-users responsability.

## 3. Guidelines

General guidelines to keep the project sane and easy-to-maintain

- Keep the README concise, bloat should be avoided at all costs.
